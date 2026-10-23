// Package model holds the status page's data types and their validation.
package model

// Impact is how badly an event affects one component.
type Impact string

const (
	Operational      Impact = "operational"
	UnderMaintenance Impact = "under_maintenance"
	Degraded         Impact = "degraded"
	PartialOutage    Impact = "partial_outage"
	MajorOutage      Impact = "major_outage"
)

var impactSeverity = map[Impact]int{
	Operational: 0, UnderMaintenance: 1, Degraded: 2, PartialOutage: 3, MajorOutage: 4,
}

var impactLabels = map[Impact]string{
	Operational:      "Operational",
	UnderMaintenance: "Under maintenance",
	Degraded:         "Degraded performance",
	PartialOutage:    "Partial outage",
	MajorOutage:      "Major outage",
}

func (i Impact) Valid() bool { _, ok := impactSeverity[i]; return ok }

// Severity orders impacts; unknown impacts sort below operational.
func (i Impact) Severity() int {
	if s, ok := impactSeverity[i]; ok {
		return s
	}
	return -1
}

// Down reports whether time at this impact counts against uptime.
func (i Impact) Down() bool { return i == PartialOutage || i == MajorOutage }

func (i Impact) Label() string {
	if l, ok := impactLabels[i]; ok {
		return l
	}
	return string(i)
}

// Worst returns the more severe of two impacts.
func Worst(a, b Impact) Impact {
	if b.Severity() > a.Severity() {
		return b
	}
	return a
}

// Kind distinguishes incidents from maintenance windows.
type Kind string

const (
	KindIncident    Kind = "incident"
	KindMaintenance Kind = "maintenance"
)

// Status is the lifecycle stage of an incident or maintenance window.
type Status string

const (
	Investigating Status = "investigating"
	Identified    Status = "identified"
	Monitoring    Status = "monitoring"
	Resolved      Status = "resolved"
	Scheduled     Status = "scheduled"
	InProgress    Status = "in_progress"
	Completed     Status = "completed"
	Cancelled     Status = "cancelled"
)

var statusLabels = map[Status]string{
	Investigating: "Investigating", Identified: "Identified", Monitoring: "Monitoring", Resolved: "Resolved",
	Scheduled: "Scheduled", InProgress: "In progress", Completed: "Completed", Cancelled: "Cancelled",
}

func (s Status) ValidFor(k Kind) bool {
	switch k {
	case KindIncident:
		return s == Investigating || s == Identified || s == Monitoring || s == Resolved
	case KindMaintenance:
		return s == Scheduled || s == InProgress || s == Completed || s == Cancelled
	}
	return false
}

func (s Status) Terminal() bool { return s == Resolved || s == Completed || s == Cancelled }

func (s Status) Label() string {
	if l, ok := statusLabels[s]; ok {
		return l
	}
	return string(s)
}
