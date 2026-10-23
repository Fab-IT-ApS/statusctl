package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"statusctl/model"
	"statusctl/ops"
	"statusctl/store"
)

// mutate opens the store, resolves the author and applies the op built for them.
func (a *app) mutate(cmd *cobra.Command, authorFlag string, build func(author string) store.Op) (err error) {
	defer func() {
		if err != nil {
			a.saveDraft()
		}
	}()
	st, p, err := a.openStore()
	if err != nil {
		return err
	}
	who, err := a.author(authorFlag, p)
	if err != nil {
		return err
	}
	res, err := st.Apply(cmd.Context(), build(who))
	if err != nil {
		return err
	}
	l := res.Incident.Latest()
	if a.jsonOut {
		return a.printJSON(map[string]string{
			"id": res.Incident.ID, "kind": string(res.Incident.Kind), "status": string(l.Status), "url": res.URL,
		})
	}
	fmt.Fprintf(a.stderr, "%s %s: %s\n%s\n", res.Incident.Kind, res.Incident.ID, l.Status.Label(), res.URL)
	return nil
}

func (a *app) incidentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "incident", Short: "Open, update and resolve incidents"}
	cmd.AddCommand(a.incidentNewCmd(), a.incidentUpdateCmd(), a.incidentResolveCmd())
	return cmd
}

func (a *app) incidentNewCmd() *cobra.Command {
	var msg msgFlags
	var comps []string
	var status, author string
	cmd := &cobra.Command{
		Use:   "new TITLE",
		Short: "Open an incident",
		Args:  exactArgs(1, "a TITLE"),
		RunE: func(cmd *cobra.Command, args []string) error {
			impacts, err := parseImpacts(comps)
			if err != nil {
				return err
			}
			text, err := a.message(msg, true)
			if err != nil {
				return err
			}
			return a.mutate(cmd, author, func(who string) store.Op {
				return func(s *model.State) (*ops.Result, error) {
					return ops.NewIncident(s, ops.NewIncidentArgs{
						Title: args[0], Status: model.Status(status), Impacts: impacts, Message: text, Author: who,
					}, a.now())
				}
			})
		},
	}
	msg.register(cmd)
	cmd.Flags().StringArrayVarP(&comps, "component", "c", nil, "affected component as ID=IMPACT (repeatable)")
	cmd.Flags().StringVar(&status, "status", "", "investigating (default), identified or monitoring")
	cmd.Flags().StringVar(&author, "author", "", "display name shown on the page")
	return cmd
}

func (a *app) incidentUpdateCmd() *cobra.Command {
	var msg msgFlags
	var comps []string
	var status, author string
	var reopen bool
	cmd := &cobra.Command{
		Use:   "update ID",
		Short: "Post an update to an incident",
		Args:  exactArgs(1, "an incident ID (or unique prefix)"),
		RunE: func(cmd *cobra.Command, args []string) error {
			impacts, err := parseImpacts(comps)
			if err != nil {
				return err
			}
			text, err := a.message(msg, true)
			if err != nil {
				return err
			}
			return a.mutate(cmd, author, func(who string) store.Op {
				return func(s *model.State) (*ops.Result, error) {
					return ops.UpdateIncident(s, ops.UpdateArgs{
						Ref: args[0], Status: model.Status(status), Impacts: impacts,
						Message: text, Author: who, Reopen: reopen,
					}, a.now())
				}
			})
		},
	}
	msg.register(cmd)
	cmd.Flags().StringArrayVarP(&comps, "component", "c", nil, "change a component's impact, as ID=IMPACT (repeatable)")
	cmd.Flags().StringVar(&status, "status", "", "investigating, identified, monitoring or resolved (default: unchanged)")
	cmd.Flags().StringVar(&author, "author", "", "display name shown on the page")
	cmd.Flags().BoolVar(&reopen, "reopen", false, "allow posting to a resolved incident")
	return cmd
}

func (a *app) incidentResolveCmd() *cobra.Command {
	var msg msgFlags
	var author string
	cmd := &cobra.Command{
		Use:   "resolve ID",
		Short: "Resolve an incident and clear all its impacts",
		Args:  exactArgs(1, "an incident ID (or unique prefix)"),
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := a.message(msg, true)
			if err != nil {
				return err
			}
			return a.mutate(cmd, author, func(who string) store.Op {
				return func(s *model.State) (*ops.Result, error) {
					return ops.ResolveIncident(s, args[0], text, who, a.now())
				}
			})
		},
	}
	msg.register(cmd)
	cmd.Flags().StringVar(&author, "author", "", "display name shown on the page")
	return cmd
}

func (a *app) maintenanceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "maintenance", Short: "Schedule, start, complete and cancel maintenance"}
	cmd.AddCommand(a.maintenanceNewCmd(),
		a.maintenanceMoveCmd("start", "Mark maintenance as in progress", ops.StartMaintenance, false),
		a.maintenanceMoveCmd("complete", "Mark maintenance as completed", ops.CompleteMaintenance, false),
		a.maintenanceMoveCmd("cancel", "Cancel scheduled maintenance", ops.CancelMaintenance, true))
	return cmd
}

func (a *app) maintenanceNewCmd() *cobra.Command {
	var msg msgFlags
	var comps []string
	var start, end, author string
	cmd := &cobra.Command{
		Use:   "new TITLE",
		Short: "Schedule maintenance",
		Args:  exactArgs(1, "a TITLE"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if start == "" || end == "" {
				return inputErrorf("--start and --end are required")
			}
			text, err := a.message(msg, false)
			if err != nil {
				return err
			}
			return a.mutate(cmd, author, func(who string) store.Op {
				return func(s *model.State) (*ops.Result, error) {
					loc := s.Config.Location()
					st, err := ops.ParseTime(start, loc)
					if err != nil {
						return nil, err
					}
					en, err := ops.ParseTime(end, loc)
					if err != nil {
						return nil, err
					}
					return ops.NewMaintenance(s, ops.NewMaintenanceArgs{
						Title: args[0], Components: comps, Start: st, End: en, Message: text, Author: who,
					}, a.now())
				}
			})
		},
	}
	msg.register(cmd)
	cmd.Flags().StringArrayVarP(&comps, "component", "c", nil, "affected component ID (repeatable)")
	cmd.Flags().StringVar(&start, "start", "", "start: RFC 3339, or YYYY-MM-DDTHH:MM in the site timezone")
	cmd.Flags().StringVar(&end, "end", "", "end: RFC 3339, or YYYY-MM-DDTHH:MM in the site timezone")
	cmd.Flags().StringVar(&author, "author", "", "display name shown on the page")
	return cmd
}

type moveFunc func(s *model.State, ref, message, author string, now time.Time) (*ops.Result, error)

func (a *app) maintenanceMoveCmd(use, short string, move moveFunc, requireMessage bool) *cobra.Command {
	var msg msgFlags
	var author string
	cmd := &cobra.Command{
		Use:   use + " ID",
		Short: short,
		Args:  exactArgs(1, "a maintenance ID (or unique prefix)"),
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := a.message(msg, requireMessage)
			if err != nil {
				return err
			}
			return a.mutate(cmd, author, func(who string) store.Op {
				return func(s *model.State) (*ops.Result, error) {
					return move(s, args[0], text, who, a.now())
				}
			})
		},
	}
	msg.register(cmd)
	cmd.Flags().StringVar(&author, "author", "", "display name shown on the page")
	return cmd
}
