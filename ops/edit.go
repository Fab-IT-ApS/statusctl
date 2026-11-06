package ops

import (
	"bytes"
	"strings"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

type EditArgs struct {
	Ref        string
	Kind       model.Kind // empty accepts either kind
	Title      string     // empty keeps the title
	Message    string     // blank keeps the latest update's message
	Start, End *time.Time // maintenance still scheduled only; nil keeps
	Author     string     // who edited; recorded in the commit only
}

// EditEvent rewrites an event's title, its latest update's message and, for
// maintenance that has not started, its schedule. The timeline is unchanged.
func EditEvent(s *model.State, a EditArgs) (*Result, error) {
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
	in := cur.Clone()
	if strings.TrimSpace(a.Title) != "" {
		if in.Title, err = checkTitle(a.Title); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(a.Message) != "" {
		if in.Updates[len(in.Updates)-1].Message, err = checkMessage(a.Message); err != nil {
			return nil, err
		}
	}
	if a.Start != nil || a.End != nil {
		if in.Kind != model.KindMaintenance {
			return nil, inputErrorf("only maintenance has start and end times")
		}
		if st := in.Latest().Status; st != model.Scheduled {
			return nil, inputErrorf("%s is %s; its start and end can only change before it starts", in.ID, st)
		}
		if a.Start != nil {
			t := stamp(*a.Start)
			in.ScheduledStart = &t
		}
		if a.End != nil {
			t := stamp(*a.End)
			in.ScheduledEnd = &t
		}
		if !in.ScheduledEnd.After(*in.ScheduledStart) {
			return nil, inputErrorf("the end must be after the start")
		}
	}
	before, err := cur.Marshal()
	if err != nil {
		return nil, err
	}
	after, err := in.Marshal()
	if err != nil {
		return nil, err
	}
	if bytes.Equal(before, after) {
		return nil, inputErrorf("nothing to change: pass a new title, message, start or end")
	}
	res, err := finish(s, in)
	if err != nil {
		return nil, err
	}
	who := strings.TrimSpace(a.Author)
	res.Action, res.CommitAuthor = "edited by "+who, who
	return res, nil
}
