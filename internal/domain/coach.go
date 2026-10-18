package domain

import "time"

// CoachClient is the coach -> client relationship.
// AssignedPlanID is the plan from the coach's library assigned to the client
// (nil = no plan assigned, the client uses their own active plan).
type CoachClient struct {
	ID             int64
	CoachID        int64
	ClientID       int64
	AssignedPlanID *int64
	AssignedAt     time.Time
}

// CoachClientInfo joins the relationship with the client's data and the
// assigned plan, for the coach dashboard.
type CoachClientInfo struct {
	CoachClient
	Client           *User
	AssignedPlanName string // empty when no plan is assigned
}

// CoachAssignment is the client-side view: who my coach is and which plan
// they assigned me.
type CoachAssignment struct {
	CoachID        int64
	CoachName      string
	AssignedPlanID *int64
}

// DemoMember is someone of a demo group (#187), for the band's "View as"
// menu: the visitor, the coach, a coach's client, or someone free to add.
type DemoMember struct {
	ID      int64
	Name    string
	Visitor bool // the one who entered the demo
	Coach   bool
	Client  bool // has a coach
}
