//go:build !race

package demo

import "time"

// startBudget is how long seeding a group may take (TestStartIsFast).
const startBudget = time.Second
