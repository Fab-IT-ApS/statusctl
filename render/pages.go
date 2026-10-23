package render

import (
	"slices"

	"statusctl/model"
)

type incidentView struct {
	eventView
	ComponentNames []string
	Timeline       []model.Update
}

func (r *renderer) incidentPages() error {
	for _, in := range r.s.Incidents {
		v := incidentView{eventView: r.event(in, "../../"), Timeline: newestFirst(in.Updates)}
		for _, id := range in.Components {
			c, _ := r.s.Config.Component(id)
			v.ComponentNames = append(v.ComponentNames, c.Name)
		}
		if err := r.page("incidents/"+in.ID+"/index.html", "incident", in.Title, "../../", v); err != nil {
			return err
		}
	}
	return nil
}

type monthView struct {
	Key, Label string
	Count      int
	Events     []eventView
}

type historyView struct {
	Months []monthView
}

func (r *renderer) history() error {
	loc := r.s.Config.Location()
	events := slices.Clone(r.s.Incidents)
	slices.SortStableFunc(events, func(a, b *model.Incident) int { return when(b).Compare(when(a)) })

	var months []*monthView
	byKey := map[string]*monthView{}
	for _, in := range events {
		t := when(in).In(loc)
		key := t.Format("2006-01")
		m, ok := byKey[key]
		if !ok {
			m = &monthView{Key: key, Label: t.Format("January 2006")}
			byKey[key] = m
			months = append(months, m)
		}
		m.Count++
		m.Events = append(m.Events, r.event(in, "../../"))
	}
	view := historyView{}
	for _, m := range months {
		if err := r.page("history/"+m.Key+"/index.html", "month", m.Label, "../../", m); err != nil {
			return err
		}
		view.Months = append(view.Months, *m)
	}
	return r.page("history/index.html", "history", "History", "../", view)
}
