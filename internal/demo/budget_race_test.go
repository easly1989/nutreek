//go:build race

package demo

import "time"

// startBudget is how long seeding a group may take (TestStartIsFast). The
// race detector makes the pure-Go SQLite about 35 times slower: a coach group
// takes about 30 ms plain and 1.2 s with -race in CI.
const startBudget = 3 * time.Second
