package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrMealTemplateNotFound viene restituito quando un template non esiste.
var ErrMealTemplateNotFound = errors.New("template non trovato")

// ErrMealTemplateForbidden segnala accesso a template altrui.
var ErrMealTemplateForbidden = errors.New("template non di proprieta' dell'utente")

// ErrSlotNotFound = slot non esistente / non appartenente all'utente.
var ErrSlotNotFound = errors.New("slot non trovato")

// MealTemplates e' il repo dei template di pasti.
type MealTemplates struct {
	db *sql.DB
}

func NewMealTemplates(db *sql.DB) *MealTemplates {
	return &MealTemplates{db: db}
}

// Create inserisce un nuovo template (vuoto, nessuna slot ancora).
func (r *MealTemplates) Create(ctx context.Context, userID int64, name string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO meal_templates (user_id, name) VALUES (?, ?)`,
		userID, strings.TrimSpace(name),
	)
	if err != nil {
		return 0, fmt.Errorf("insert meal_template: %w", err)
	}
	return res.LastInsertId()
}

// Rename cambia solo il nome.
func (r *MealTemplates) Rename(ctx context.Context, id, userID int64, name string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE meal_templates SET name = ? WHERE id = ? AND user_id = ?`,
		strings.TrimSpace(name), id, userID,
	)
	if err != nil {
		return fmt.Errorf("rename meal_template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return r.notFoundOrForbidden(ctx, id, userID)
	}
	return nil
}

// Delete cancella template + slots (CASCADE).
func (r *MealTemplates) Delete(ctx context.Context, id, userID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM meal_templates WHERE id = ? AND user_id = ?`, id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete meal_template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return r.notFoundOrForbidden(ctx, id, userID)
	}
	return nil
}

// SetActive attiva il template e disattiva tutti gli altri dello stesso
// utente, in transazione: serve a mantenere l'invariante "max 1 attivo".
func (r *MealTemplates) SetActive(ctx context.Context, id, userID int64) error {
	if err := r.assertOwner(ctx, id, userID); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`UPDATE meal_templates SET is_active = 0 WHERE user_id = ?`, userID,
	); err != nil {
		return fmt.Errorf("clear active: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE meal_templates SET is_active = 1 WHERE id = ? AND user_id = ?`, id, userID,
	); err != nil {
		return fmt.Errorf("set active: %w", err)
	}
	return tx.Commit()
}

// ListByUser ritorna i template dell'utente, con il piu' recente prima.
func (r *MealTemplates) ListByUser(ctx context.Context, userID int64) ([]*domain.MealTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, is_active, created_at
		FROM meal_templates
		WHERE user_id = ?
		ORDER BY is_active DESC, created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()
	out := make([]*domain.MealTemplate, 0, 4)
	for rows.Next() {
		var t domain.MealTemplate
		var act int
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &act, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		t.IsActive = act == 1
		out = append(out, &t)
	}
	return out, rows.Err()
}

// GetWithSlots carica template + slot ordinate per (day_of_week, sort_order).
func (r *MealTemplates) GetWithSlots(ctx context.Context, id, userID int64) (*domain.MealTemplateWithSlots, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, is_active, created_at
		 FROM meal_templates WHERE id = ?`, id,
	)
	var (
		out domain.MealTemplateWithSlots
		act int
	)
	err := row.Scan(&out.ID, &out.UserID, &out.Name, &act, &out.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMealTemplateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan template: %w", err)
	}
	if out.UserID != userID {
		return nil, ErrMealTemplateForbidden
	}
	out.IsActive = act == 1

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, template_id, day_of_week, name, sort_order
		FROM meal_template_slots WHERE template_id = ?
		ORDER BY day_of_week, sort_order, id
	`, id)
	if err != nil {
		return nil, fmt.Errorf("list slots: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s domain.MealTemplateSlot
		if err := rows.Scan(&s.ID, &s.TemplateID, &s.DayOfWeek, &s.Name, &s.SortOrder); err != nil {
			return nil, fmt.Errorf("scan slot: %w", err)
		}
		out.Slots = append(out.Slots, s)
	}
	return &out, rows.Err()
}

// AddSlot aggiunge una slot al template. day deve essere -1..6.
func (r *MealTemplates) AddSlot(ctx context.Context, templateID, userID int64, day int, name string) (int64, error) {
	if err := r.assertOwner(ctx, templateID, userID); err != nil {
		return 0, err
	}
	if day < -1 || day > 6 {
		return 0, fmt.Errorf("day_of_week fuori range")
	}
	if strings.TrimSpace(name) == "" {
		return 0, fmt.Errorf("nome slot vuoto")
	}
	var nextOrder int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order)+1, 0) FROM meal_template_slots
		 WHERE template_id = ? AND day_of_week = ?`,
		templateID, day,
	).Scan(&nextOrder); err != nil {
		return 0, fmt.Errorf("compute sort_order: %w", err)
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO meal_template_slots (template_id, day_of_week, name, sort_order)
		VALUES (?, ?, ?, ?)`,
		templateID, day, strings.TrimSpace(name), nextOrder,
	)
	if err != nil {
		return 0, fmt.Errorf("insert slot: %w", err)
	}
	return res.LastInsertId()
}

// RemoveSlot cancella una slot, verificando ownership tramite join.
func (r *MealTemplates) RemoveSlot(ctx context.Context, slotID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM meal_template_slots
		WHERE id = ?
		  AND template_id IN (SELECT id FROM meal_templates WHERE user_id = ?)
	`, slotID, userID)
	if err != nil {
		return fmt.Errorf("delete slot: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSlotNotFound
	}
	return nil
}

// ---- helpers ---------------------------------------------------------------

func (r *MealTemplates) assertOwner(ctx context.Context, id, userID int64) error {
	var owner int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM meal_templates WHERE id = ?`, id).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMealTemplateNotFound
	}
	if err != nil {
		return fmt.Errorf("assert owner: %w", err)
	}
	if owner != userID {
		return ErrMealTemplateForbidden
	}
	return nil
}

func (r *MealTemplates) notFoundOrForbidden(ctx context.Context, id, userID int64) error {
	return r.assertOwner(ctx, id, userID)
}
