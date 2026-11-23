package stores

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// An integrated store runs in this process: its answer is read like a network one, its size
// is capped, and a panic in it is a failed call.
func TestBuiltinCall(t *testing.T) {
	c := NewClient(true)
	c.Builtin = map[string]http.Handler{
		"ok": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/search" || r.URL.Query().Get("q") != "pasta" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"products":[{"id":"1","name":"Pasta","nutrition":{"kcal":350,"protein":12,"carbs":72,"fat":1.5}}]}`))
		}),
		"huge": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", maxBody+10))) }),
		"boom": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("parser bug") }),
		"split": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { // answers in pieces that add up to too much
			for i := 0; i < 4; i++ {
				if _, err := w.Write([]byte(strings.Repeat("x", maxBody/3))); err != nil {
					return
				}
			}
		}),
	}
	ctx := context.Background()
	got, err := c.Search(ctx, BuiltinURL("ok"), "", "pasta")
	if err != nil || len(got) != 1 || got[0].Name != "Pasta" || got[0].Kcal != 350 {
		t.Fatalf("Search = %+v, %v", got, err)
	}
	if _, err := c.Search(ctx, BuiltinURL("nope"), "", "pasta"); err == nil {
		t.Error("an unknown integrated store must fail, not reach the network")
	}
	for _, id := range []string{"huge", "split"} {
		if _, err := c.Search(ctx, BuiltinURL(id), "", "pasta"); err == nil {
			t.Errorf("%s: an answer over the cap must fail", id)
		}
	}
	if _, err := c.Search(ctx, BuiltinURL("boom"), "", "pasta"); err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Errorf("a panic in the store = %v, want an error", err)
	}
	if !IsBuiltin("builtin://conad") || IsBuiltin("https://builtin://x") || IsBuiltin("builtin://") {
		t.Error("IsBuiltin")
	}
	if _, err := CleanURL("builtin://conad", true); err == nil {
		t.Error("a form must not be able to store a builtin address")
	}
	_ = url.Values{}
}
