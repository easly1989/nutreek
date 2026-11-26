package app

import (
	"os"
	"testing"

	"github.com/amon2126/nutreek/internal/auth"
)

// TestMain makes password hashing fast: these tests hash a password for nearly every account
// they make, and under -race that is the longest part of the run.
func TestMain(m *testing.M) {
	auth.UseMinimumCostForTests()
	os.Exit(m.Run())
}
