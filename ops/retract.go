package ops

import (
	"strings"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

// RetractWindow is how long after closing an event can still be retracted
// without Force; retracting older events would rewrite history and uptime.
const RetractWindow = 7 * 24 * time.Hour

type RetractArgs struct {
	Ref    string
	Kind   model.Kind // empty accepts either kind
	Author string     // who retracted; recorded in the commit only
	Force  bool       // allow events closed longer than RetractWindow ago
}

// RetractEvent removes an event from the repository. The result's Incident is
// the event as it was, so callers can describe what they removed.
func RetractEvent(s *model.State, a RetractArgs, now time.Time) (*Result, error) {
	cur, err := Find(s, a.Ref)
	if err != nil {
		return nil, err
	}
	if err := checkKind(cur, a.Kind); err != nil {
		return nil, err
	}
	if err := checkAuthor(a.Author); err != nil {
		return nil, err
	}
	if cur.Latest().Status.Terminal() && now.Sub(cur.LastUpdated()) > RetractWindow && !a.Force {
		return nil, inputErrorf("%s closed more than 7 days ago; retracting it would rewrite history (pass --force to do it anyway)", cur.ID)
	}
	who := strings.TrimSpace(a.Author)
	return &Result{
		Incident:     cur.Clone(),
		URL:          strings.TrimRight(s.Config.Site.URL, "/") + "/",
		Deleted:      []string{cur.Path()},
		Action:       "retracted by " + who,
		CommitAuthor: who,
	}, nil
}
