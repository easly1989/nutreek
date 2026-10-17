package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The demo's code is in the demo build only (#137): the normal binary does
// not even link the package.
func TestDemoOnlyInDemoBuild(t *testing.T) {
	const pkg = "github.com/amon2126/nutreek/internal/demo"
	deps := func(args ...string) string {
		out, err := exec.Command("go", append([]string{"list", "-deps"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("go list %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if strings.Contains(deps("."), pkg) {
		t.Error("the normal build links the demo")
	}
	if !strings.Contains(deps("-tags", "demo", "."), pkg) {
		t.Error("the demo build does not link the demo")
	}
}
