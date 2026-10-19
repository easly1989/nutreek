package db

import (
	"context"
	"testing"
)

func TestInstanceSettings(t *testing.T) {
	ctx := context.Background()
	s := NewInstanceSettings(newTestStore(t).DB)
	if _, ok, err := s.Get(ctx, "update_check"); err != nil || ok {
		t.Fatalf("unset key: ok=%v err=%v", ok, err)
	}
	for _, v := range []string{"0", "1"} {
		if err := s.Set(ctx, "update_check", v); err != nil {
			t.Fatal(err)
		}
		if got, ok, err := s.Get(ctx, "update_check"); err != nil || !ok || got != v {
			t.Errorf("after Set %q: %q ok=%v err=%v", v, got, ok, err)
		}
	}
}
