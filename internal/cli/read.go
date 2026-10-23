package cli

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/Fab-IT-ApS/statusctl/derive"
	"github.com/Fab-IT-ApS/statusctl/model"
	"github.com/Fab-IT-ApS/statusctl/ops"
	"github.com/Fab-IT-ApS/statusctl/render"
)

func (a *app) validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check status.yaml and every incident file (local directory)",
		Args:  exactArgs(0, "no arguments"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := model.Load(os.DirFS(a.localDir()))
			if err != nil {
				return err
			}
			fmt.Fprintf(a.stderr, "ok: %d components, %d incidents and maintenance windows\n",
				len(s.Config.Components), len(s.Incidents))
			return nil
		},
	}
}

func (a *app) renderCmd() *cobra.Command {
	var out, nowFlag string
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Build the static site from a local directory",
		Args:  exactArgs(0, "no arguments"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if out == "" {
				return &ops.InputError{Msg: "--out is required"}
			}
			now := a.now()
			if nowFlag != "" {
				t, err := time.Parse(time.RFC3339, nowFlag)
				if err != nil {
					return &ops.InputError{Msg: "--now must be RFC 3339, e.g. 2026-10-23T12:00:00Z"}
				}
				now = t
			}
			fsys := os.DirFS(a.localDir())
			s, err := model.Load(fsys)
			if err != nil {
				return err
			}
			files, err := render.Render(fsys, s, now)
			if err != nil {
				return err
			}
			for _, p := range slices.Sorted(maps.Keys(files)) {
				full := filepath.Join(out, filepath.FromSlash(p))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(full, files[p], 0o644); err != nil {
					return err
				}
			}
			fmt.Fprintf(a.stderr, "rendered %d files to %s\n", len(files), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output directory")
	cmd.Flags().StringVar(&nowFlag, "now", "", "render as if at this RFC 3339 time")
	return cmd
}

func (a *app) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current derived status of every component",
		Args:  exactArgs(0, "no arguments"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, _, err := a.openStore()
			if err != nil {
				return err
			}
			s, err := st.Load(cmd.Context())
			if err != nil {
				return err
			}
			statuses := derive.ComponentStatuses(s)
			type comp struct {
				ID     string       `json:"id"`
				Name   string       `json:"name"`
				Group  string       `json:"group,omitempty"`
				Status model.Impact `json:"status"`
			}
			comps := make([]comp, 0)
			for _, c := range s.Config.ActiveComponents() {
				comps = append(comps, comp{c.ID, c.Name, c.Group, statuses[c.ID]})
			}
			overall := derive.Overall(s)
			if a.jsonOut {
				return a.printJSON(map[string]any{"status": overall, "components": comps})
			}
			fmt.Fprintln(a.stdout, overall.Label())
			tw := tabwriter.NewWriter(a.stdout, 0, 4, 2, ' ', 0)
			for _, c := range comps {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", c.ID, c.Status.Label(), c.Name)
			}
			return tw.Flush()
		},
	}
}

func (a *app) listCmd() *cobra.Command {
	var open bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List incidents and maintenance windows, newest first",
		Args:  exactArgs(0, "no arguments"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, _, err := a.openStore()
			if err != nil {
				return err
			}
			s, err := st.Load(cmd.Context())
			if err != nil {
				return err
			}
			type item struct {
				ID        string       `json:"id"`
				Kind      model.Kind   `json:"kind"`
				Title     string       `json:"title"`
				Status    model.Status `json:"status"`
				StartedAt time.Time    `json:"started_at"`
				UpdatedAt time.Time    `json:"updated_at"`
			}
			events := slices.Clone(s.Incidents)
			slices.SortStableFunc(events, func(x, y *model.Incident) int { return y.Started().Compare(x.Started()) })
			items := make([]item, 0)
			for _, in := range events {
				if open && in.Latest().Status.Terminal() {
					continue
				}
				items = append(items, item{in.ID, in.Kind, in.Title, in.Latest().Status, in.Started(), in.LastUpdated()})
			}
			if a.jsonOut {
				return a.printJSON(items)
			}
			tw := tabwriter.NewWriter(a.stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tKIND\tSTATUS\tTITLE")
			for _, it := range items {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", it.ID, it.Kind, it.Status, it.Title)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&open, "open", false, "only events that are not resolved, completed or cancelled")
	return cmd
}
