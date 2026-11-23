package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Store plugins (#239): the services a user connects to search a store, where
// OpenFoodFacts stands among the search sources, and the store product an
// ingredient was made from. See migration 0048 and internal/stores.

// MaxStorePlugins is how many plugins a user can connect.
const MaxStorePlugins = 5

// ErrInvalidStore is returned for a name or address that cannot be saved, or a sixth plugin.
var ErrInvalidStore = errors.New("invalid store plugin")

// ErrStoreNotFound is returned for a plugin that is not the user's.
var ErrStoreNotFound = errors.New("store plugin not found")

// OFFKey is the name of OpenFoodFacts in the order of the search sources.
const OFFKey = "off"

// StorePlugin is a connected store. TokenSealed is the token as stored: open it with
// stores.Box.
type StorePlugin struct {
	ID          int64
	UserID      int64
	Name        string
	BaseURL     string
	TokenSealed string
	Enabled     bool
	Position    int
}

// SearchSource is one place a search looks in, in the user's order: OpenFoodFacts
// (OFF) or a plugin.
type SearchSource struct {
	OFF     bool
	Enabled bool
	Plugin  StorePlugin // zero for OFF
}

// Key is the source's name in an order: "off" or the plugin's id.
func (s SearchSource) Key() string {
	if s.OFF {
		return OFFKey
	}
	return strconv.FormatInt(s.Plugin.ID, 10)
}

// StoreLink is the store product behind an ingredient, with what was last known of
// its price and availability.
type StoreLink struct {
	UserID, IngredientID, PluginID int64
	PluginName                     string // filled on reads
	ProductID, ProductName         string
	SizeQty                        float64
	SizeUnit                       string
	PriceCents                     int
	HasPrice                       bool
	Currency                       string
	Available                      bool
	CheckedAt                      time.Time
}

// StorePlugins is the repository of the store tables.
type StorePlugins struct {
	db *sql.DB
}

// NewStorePlugins returns the repository.
func NewStorePlugins(db *sql.DB) *StorePlugins { return &StorePlugins{db: db} }

const storePluginCols = `id, user_id, name, base_url, token_sealed, enabled, position`

func scanPlugin(sc interface{ Scan(...any) error }) (StorePlugin, error) {
	var p StorePlugin
	err := sc.Scan(&p.ID, &p.UserID, &p.Name, &p.BaseURL, &p.TokenSealed, &p.Enabled, &p.Position)
	return p, err
}

// List returns the user's plugins in their order.
func (s *StorePlugins) List(ctx context.Context, userID int64) ([]StorePlugin, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+storePluginCols+` FROM store_plugins WHERE user_id = ? ORDER BY position, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list store plugins: %w", err)
	}
	defer rows.Close()
	var out []StorePlugin
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns one of the user's plugins.
func (s *StorePlugins) Get(ctx context.Context, userID, id int64) (StorePlugin, error) {
	p, err := scanPlugin(s.db.QueryRowContext(ctx, `SELECT `+storePluginCols+` FROM store_plugins WHERE user_id = ? AND id = ?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return StorePlugin{}, ErrStoreNotFound
	}
	return p, err
}

// Sources returns where a search looks, in the user's order, OpenFoodFacts among the
// plugins at the place the user gave it.
func (s *StorePlugins) Sources(ctx context.Context, userID int64) ([]SearchSource, error) {
	plugins, err := s.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	var rank int
	var offOn bool
	if err := s.db.QueryRowContext(ctx, `SELECT off_rank, off_enabled FROM users WHERE id = ?`, userID).Scan(&rank, &offOn); err != nil {
		return nil, err
	}
	rank = max(0, min(rank, len(plugins)))
	out := make([]SearchSource, 0, len(plugins)+1)
	for i, p := range plugins {
		if i == rank {
			out = append(out, SearchSource{OFF: true, Enabled: offOn})
		}
		out = append(out, SearchSource{Enabled: p.Enabled, Plugin: p})
	}
	if rank == len(plugins) {
		out = append(out, SearchSource{OFF: true, Enabled: offOn})
	}
	return out, nil
}

func validStore(name, baseURL string) (string, bool) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	return name, n >= 1 && n <= 60 && baseURL != ""
}

// Add connects a plugin: it goes last, after OpenFoodFacts when that was last too.
func (s *StorePlugins) Add(ctx context.Context, userID int64, name, baseURL, tokenSealed string) (int64, error) {
	name, ok := validStore(name, baseURL)
	if !ok || tokenSealed == "" {
		return 0, ErrInvalidStore
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var have, rank int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM store_plugins WHERE user_id = ?`, userID).Scan(&have); err != nil {
		return 0, err
	}
	if have >= MaxStorePlugins {
		return 0, ErrInvalidStore
	}
	if err := tx.QueryRowContext(ctx, `SELECT off_rank FROM users WHERE id = ?`, userID).Scan(&rank); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO store_plugins (user_id, name, base_url, token_sealed, position)
		 VALUES (?, ?, ?, ?, COALESCE((SELECT MAX(position) + 1 FROM store_plugins WHERE user_id = ?), 0))`,
		userID, name, baseURL, tokenSealed, userID)
	if err != nil {
		return 0, err
	}
	if rank >= have { // OpenFoodFacts was last: the new one goes after it
		rank = have
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET off_rank = ? WHERE id = ?`, rank, userID); err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// Update changes a plugin's name and address, and its token when tokenSealed is not empty.
func (s *StorePlugins) Update(ctx context.Context, userID, id int64, name, baseURL, tokenSealed string) error {
	name, ok := validStore(name, baseURL)
	if !ok {
		return ErrInvalidStore
	}
	q, args := `UPDATE store_plugins SET name = ?, base_url = ? WHERE user_id = ? AND id = ?`, []any{name, baseURL, userID, id}
	if tokenSealed != "" {
		q, args = `UPDATE store_plugins SET name = ?, base_url = ?, token_sealed = ? WHERE user_id = ? AND id = ?`, []any{name, baseURL, tokenSealed, userID, id}
	}
	return affectOne(s.db.ExecContext(ctx, q, args...))
}

// SetEnabled turns a plugin on or off.
func (s *StorePlugins) SetEnabled(ctx context.Context, userID, id int64, on bool) error {
	return affectOne(s.db.ExecContext(ctx, `UPDATE store_plugins SET enabled = ? WHERE user_id = ? AND id = ?`, on, userID, id))
}

// SetOFFEnabled turns OpenFoodFacts on or off as a search source.
func (s *StorePlugins) SetOFFEnabled(ctx context.Context, userID int64, on bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET off_enabled = ? WHERE id = ?`, on, userID)
	return err
}

// Delete disconnects a plugin; the ingredients made from its products stay, without the link.
func (s *StorePlugins) Delete(ctx context.Context, userID, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var pos int
	if err := tx.QueryRowContext(ctx, `SELECT position FROM store_plugins WHERE user_id = ? AND id = ?`, userID, id).Scan(&pos); errors.Is(err, sql.ErrNoRows) {
		return ErrStoreNotFound
	} else if err != nil {
		return err
	}
	var before int // plugins that stood before it: OpenFoodFacts moves up when it was after
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM store_plugins WHERE user_id = ? AND (position < ? OR (position = ? AND id < ?))`, userID, pos, pos, id).Scan(&before); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM store_plugins WHERE user_id = ? AND id = ?`, userID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET off_rank = MAX(0, off_rank - 1) WHERE id = ? AND off_rank > ?`, userID, before); err != nil {
		return err
	}
	return tx.Commit()
}

// Reorder saves the order of the sources: keys are "off" and the ids of the user's
// plugins. Unknown keys are ignored and the plugins left out go last, so a stale
// page cannot lose one.
func (s *StorePlugins) Reorder(ctx context.Context, userID int64, keys []string) error {
	plugins, err := s.List(ctx, userID)
	if err != nil {
		return err
	}
	byID := make(map[string]int64, len(plugins))
	for _, p := range plugins {
		byID[strconv.FormatInt(p.ID, 10)] = p.ID
	}
	var order []int64
	rank, seenOFF := -1, false
	for _, k := range keys {
		switch id, isPlugin := byID[k]; {
		case k == OFFKey && !seenOFF:
			seenOFF, rank = true, len(order)
		case isPlugin:
			order = append(order, id)
			delete(byID, k)
		}
	}
	for _, p := range plugins { // left out
		if _, left := byID[strconv.FormatInt(p.ID, 10)]; left {
			order = append(order, p.ID)
		}
	}
	if rank < 0 {
		rank = len(order)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range order {
		if _, err := tx.ExecContext(ctx, `UPDATE store_plugins SET position = ? WHERE user_id = ? AND id = ?`, i, userID, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET off_rank = ? WHERE id = ?`, rank, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func affectOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStoreNotFound
	}
	return nil
}

// ---- the product behind an ingredient ---------------------------------------

const linkCols = `l.user_id, l.ingredient_id, l.plugin_id, p.name, l.product_id, l.product_name,
	l.size_qty, l.size_unit, l.price_cents, l.currency, l.available, l.checked_at`

func scanLink(sc interface{ Scan(...any) error }) (StoreLink, error) {
	var l StoreLink
	var price sql.NullInt64
	err := sc.Scan(&l.UserID, &l.IngredientID, &l.PluginID, &l.PluginName, &l.ProductID, &l.ProductName,
		&l.SizeQty, &l.SizeUnit, &price, &l.Currency, &l.Available, &l.CheckedAt)
	l.PriceCents, l.HasPrice = int(price.Int64), price.Valid
	return l, err
}

// SaveLink records the product an ingredient was made from.
func (s *StorePlugins) SaveLink(ctx context.Context, l StoreLink) error {
	var price any
	if l.HasPrice {
		price = l.PriceCents
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ingredient_store_products (user_id, ingredient_id, plugin_id, product_id, product_name, size_qty, size_unit, price_cents, currency, available)
		 SELECT ?, ?, id, ?, ?, ?, ?, ?, ?, ? FROM store_plugins WHERE id = ? AND user_id = ?
		 ON CONFLICT (user_id, ingredient_id) DO UPDATE SET plugin_id = excluded.plugin_id, product_id = excluded.product_id,
		   product_name = excluded.product_name, size_qty = excluded.size_qty, size_unit = excluded.size_unit,
		   price_cents = excluded.price_cents, currency = excluded.currency, available = excluded.available, checked_at = CURRENT_TIMESTAMP`,
		l.UserID, l.IngredientID, l.ProductID, l.ProductName, l.SizeQty, l.SizeUnit, price, l.Currency, l.Available, l.PluginID, l.UserID)
	return err
}

// LinkedIngredient returns the ingredient a plugin's product was made into, 0 when none.
func (s *StorePlugins) LinkedIngredient(ctx context.Context, userID, pluginID int64, productID string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT ingredient_id FROM ingredient_store_products WHERE user_id = ? AND plugin_id = ? AND product_id = ?`, userID, pluginID, productID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// Links returns the links of the user's ingredients, by ingredient id.
func (s *StorePlugins) Links(ctx context.Context, userID int64, ingredientIDs []int64) (map[int64]StoreLink, error) {
	out := make(map[int64]StoreLink, len(ingredientIDs))
	if len(ingredientIDs) == 0 {
		return out, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ingredientIDs)), ",")
	args := []any{userID}
	for _, id := range ingredientIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+linkCols+` FROM ingredient_store_products l JOIN store_plugins p ON p.id = l.plugin_id
		 WHERE l.user_id = ? AND l.ingredient_id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("store links: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out[l.IngredientID] = l
	}
	return out, rows.Err()
}

// LinksOf returns every link of a plugin: what a price refresh asks about.
func (s *StorePlugins) LinksOf(ctx context.Context, userID, pluginID int64) ([]StoreLink, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+linkCols+` FROM ingredient_store_products l JOIN store_plugins p ON p.id = l.plugin_id
		 WHERE l.user_id = ? AND l.plugin_id = ?`, userID, pluginID)
	if err != nil {
		return nil, fmt.Errorf("store links: %w", err)
	}
	defer rows.Close()
	var out []StoreLink
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SetStatus stores a refreshed price and availability for a plugin's product.
func (s *StorePlugins) SetStatus(ctx context.Context, userID, pluginID int64, productID string, priceCents int, hasPrice, available bool) error {
	var price any
	if hasPrice {
		price = priceCents
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE ingredient_store_products SET price_cents = ?, available = ?, checked_at = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND plugin_id = ? AND product_id = ?`, price, available, userID, pluginID, productID)
	return err
}
