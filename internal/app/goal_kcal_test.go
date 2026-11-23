package app

import (
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

// goalKcalFor raises a day's goal by the kcal its workouts burned only when the
// user asked for it (#251).
func TestGoalKcalFor(t *testing.T) {
	target := 2000.0
	off := &domain.User{TargetKcal: &target}
	on := &domain.User{TargetKcal: &target, WorkoutsInGoal: true}

	if g := goalKcalFor(off, 300); g == nil || *g != 2000 {
		t.Errorf("setting off: %v, want 2000", g)
	}
	if g := goalKcalFor(on, 300); g == nil || *g != 2300 {
		t.Errorf("setting on: %v, want 2300", g)
	}
	if *on.TargetKcal != 2000 {
		t.Error("the profile's goal was changed")
	}
	if g := goalKcalFor(on, 0); g == nil || *g != 2000 {
		t.Errorf("no workout: %v, want 2000", g)
	}
	if g := goalKcalFor(&domain.User{WorkoutsInGoal: true}, 300); g != nil {
		t.Errorf("no goal set: %v, want nil", g)
	}
	if g := goalKcalFor(nil, 300); g != nil {
		t.Errorf("no user: %v, want nil", g)
	}
}
