package db

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// Store plugins (#239): connected per user, ordered together with OpenFoodFacts, and
// the product an ingredient was made from.
func TestStorePlugins(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	sp := NewStorePlugins(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")

	// Nothing connected: a search looks in OpenFoodFacts only.
	if src, _ := sp.Sources(ctx, alice); len(src) != 1 || !src[0].OFF || !src[0].Enabled {
		t.Fatalf("Sources with no plugin = %+v", src)
	}

	// Connect two; validation; the limit.
	conad, err := sp.Add(ctx, alice, "  Conad  ", "https://a.example", "sealed")
	if err != nil {
		t.Fatal(err)
	}
	lidl, _ := sp.Add(ctx, alice, "Lidl", "https://b.example", "sealed")
	for _, bad := range [][3]string{{"", "https://a.example", "s"}, {strings.Repeat("x", 61), "https://a.example", "s"}, {"X", "", "s"}, {"X", "https://a.example", ""}} {
		if _, err := sp.Add(ctx, alice, bad[0], bad[1], bad[2]); !errors.Is(err, ErrInvalidStore) {
			t.Errorf("Add(%q) = %v, want ErrInvalidStore", bad, err)
		}
	}
	for i := 2; i < MaxStorePlugins; i++ {
		if _, err := sp.Add(ctx, alice, "S", "https://c.example", "sealed"); err != nil {
			t.Fatalf("plugin %d: %v", i, err)
		}
	}
	if _, err := sp.Add(ctx, alice, "One too many", "https://c.example", "sealed"); !errors.Is(err, ErrInvalidStore) {
		t.Errorf("a sixth plugin = %v", err)
	}
	if p, _ := sp.Get(ctx, alice, conad); p.Name != "Conad" || p.TokenSealed != "sealed" {
		t.Errorf("Get = %+v", p)
	}

	// Scoped to the user.
	if _, err := sp.Get(ctx, bob, conad); !errors.Is(err, ErrStoreNotFound) {
		t.Errorf("Bob reading Alice's plugin = %v", err)
	}
	if err := sp.Update(ctx, bob, conad, "Mine", "https://x.example", ""); !errors.Is(err, ErrStoreNotFound) {
		t.Errorf("Bob changing it = %v", err)
	}
	if err := sp.SetEnabled(ctx, bob, conad, false); !errors.Is(err, ErrStoreNotFound) {
		t.Errorf("Bob turning it off = %v", err)
	}
	if err := sp.Delete(ctx, bob, conad); !errors.Is(err, ErrStoreNotFound) {
		t.Errorf("Bob deleting it = %v", err)
	}

	// Update keeps the token when none is given.
	if err := sp.Update(ctx, alice, conad, "Conad Mestre", "https://a2.example", ""); err != nil {
		t.Fatal(err)
	}
	if p, _ := sp.Get(ctx, alice, conad); p.Name != "Conad Mestre" || p.BaseURL != "https://a2.example" || p.TokenSealed != "sealed" {
		t.Errorf("after the update: %+v", p)
	}
	if err := sp.Update(ctx, alice, conad, "Conad Mestre", "https://a2.example", "new"); err != nil {
		t.Fatal(err)
	}
	if p, _ := sp.Get(ctx, alice, conad); p.TokenSealed != "new" {
		t.Errorf("the token was not replaced: %+v", p)
	}

	// The order: OpenFoodFacts starts first; the user can put it anywhere, and a stale list loses nobody.
	keys := func() []string {
		src, _ := sp.Sources(ctx, alice)
		var out []string
		for _, s := range src {
			out = append(out, s.Key())
		}
		return out
	}
	if k := keys(); k[0] != OFFKey || len(k) != MaxStorePlugins+1 {
		t.Fatalf("default order = %v", k)
	}
	conadKey, lidlKey := idKey(conad), idKey(lidl)
	if err := sp.Reorder(ctx, alice, []string{conadKey, OFFKey, lidlKey, "999", "junk", conadKey}); err != nil {
		t.Fatal(err)
	}
	if k := keys(); k[0] != conadKey || k[1] != OFFKey || k[2] != lidlKey || len(k) != MaxStorePlugins+1 {
		t.Errorf("order after Reorder = %v", k)
	}
	if err := sp.Reorder(ctx, alice, []string{lidlKey}); err != nil { // OFF and the others left out: they go last
		t.Fatal(err)
	}
	if k := keys(); k[0] != lidlKey || k[len(k)-1] != OFFKey || len(k) != MaxStorePlugins+1 {
		t.Errorf("a short list = %v", k)
	}
	_ = sp.SetOFFEnabled(ctx, alice, false)
	_ = sp.SetEnabled(ctx, alice, lidl, false)
	src, _ := sp.Sources(ctx, alice)
	if src[0].Enabled || src[len(src)-1].Enabled {
		t.Errorf("switches: %+v", src)
	}
	if other, _ := sp.Sources(ctx, bob); len(other) != 1 || !other[0].Enabled {
		t.Errorf("Bob's sources changed with Alice's: %+v", other)
	}

	// Links: one product for each ingredient, scoped to the user, dropped with the plugin.
	oats, _ := ings.CreateLocal(ctx, "Oats", nil, "g", 380, 13, 60, 7, &alice, nil, nil, nil, nil, nil, nil)
	link := StoreLink{UserID: alice, IngredientID: oats, PluginID: conad, ProductID: "p1", ProductName: "Fiocchi", SizeQty: 500, SizeUnit: "g", PriceCents: 189, HasPrice: true, Currency: "EUR", Available: true}
	if err := sp.SaveLink(ctx, link); err != nil {
		t.Fatal(err)
	}
	if id, _ := sp.LinkedIngredient(ctx, alice, conad, "p1"); id != oats {
		t.Errorf("LinkedIngredient = %d", id)
	}
	if id, _ := sp.LinkedIngredient(ctx, bob, conad, "p1"); id != 0 {
		t.Errorf("Bob sees Alice's link: %d", id)
	}
	link.UserID = bob // Bob cannot link to Alice's plugin
	if err := sp.SaveLink(ctx, link); err != nil {
		t.Fatal(err)
	}
	if l, _ := sp.Links(ctx, bob, []int64{oats}); len(l) != 0 {
		t.Errorf("a link to another user's plugin was saved: %+v", l)
	}
	got, _ := sp.Links(ctx, alice, []int64{oats, 12345})
	if l := got[oats]; len(got) != 1 || l.PriceCents != 189 || l.PluginName != "Conad Mestre" || !l.Available {
		t.Errorf("Links = %+v", got)
	}
	if err := sp.SetStatus(ctx, alice, conad, "p1", 0, false, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := sp.Links(ctx, alice, []int64{oats}); got[oats].HasPrice || got[oats].Available {
		t.Errorf("after SetStatus: %+v", got[oats])
	}
	link.UserID, link.PluginID, link.ProductID = alice, conad, "p2" // another product for the same ingredient replaces the first
	_ = sp.SaveLink(ctx, link)
	if id, _ := sp.LinkedIngredient(ctx, alice, conad, "p1"); id != 0 {
		t.Errorf("the replaced product is still linked: %d", id)
	}

	// Disconnecting drops the plugin's links and moves OpenFoodFacts up when it was after it.
	if err := sp.Delete(ctx, alice, conad); err != nil {
		t.Fatal(err)
	}
	if l, _ := sp.Links(ctx, alice, []int64{oats}); len(l) != 0 {
		t.Errorf("links left after disconnecting: %+v", l)
	}
	if k := keys(); len(k) != MaxStorePlugins || k[len(k)-1] != OFFKey {
		t.Errorf("order after Delete = %v", k)
	}
	if err := sp.Delete(ctx, alice, conad); !errors.Is(err, ErrStoreNotFound) {
		t.Errorf("deleting twice = %v", err)
	}
}

func idKey(id int64) string { return strconv.FormatInt(id, 10) }
