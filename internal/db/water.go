package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrWaterLogNotFound viene restituito quando non c'e' un log da cancellare.
var ErrWaterLogNotFound = errors.New("log idratazione non trovato")

type WaterLogs struct {
	db *sql.DB
}

func NewWaterLogs(db *sql.DB) *WaterLogs {
	return &WaterLogs{db: db}
}

// Log registra un'assunzione d'acqua per la data data. Ogni chiamata e' un
// nuovo record (i quick-log "+250ml" si sommano nel totale giornaliero).
func (w *WaterLogs) Log(ctx context.Context, userID int64, date time.Time, amountML int) error {
	if amountML <= 0 {
		return fmt.Errorf("quantita' non valida")
	}
	_, err := w.db.ExecContext(ctx,
		`INSERT INTO water_logs (user_id, logged_at, amount_ml) VALUES (?, ?, ?)`,
		userID, date.Format("2006-01-02"), amountML,
	)
	if err != nil {
		return fmt.Errorf("insert water log: %w", err)
	}
	return nil
}

// TotalForDate ritorna i ml totali bevuti dall'utente in una data.
func (w *WaterLogs) TotalForDate(ctx context.Context, userID int64, date time.Time) (int, error) {
	var total int
	err := w.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount_ml), 0) FROM water_logs WHERE user_id = ? AND logged_at = ?`,
		userID, date.Format("2006-01-02"),
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("total water for date: %w", err)
	}
	return total, nil
}

// ForDateRange ritorna una mappa data(YYYY-MM-DD) -> ml totali per l'intervallo
// [from, to] inclusivo. Le date senza log non compaiono nella mappa.
func (w *WaterLogs) ForDateRange(ctx context.Context, userID int64, from, to time.Time) (map[string]int, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT logged_at, SUM(amount_ml) FROM water_logs
		 WHERE user_id = ? AND logged_at BETWEEN ? AND ?
		 GROUP BY logged_at`,
		userID, from.Format("2006-01-02"), to.Format("2006-01-02"),
	)
	if err != nil {
		return nil, fmt.Errorf("water for range: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int)
	for rows.Next() {
		var day string
		var total int
		if err := rows.Scan(&day, &total); err != nil {
			return nil, fmt.Errorf("scan water range: %w", err)
		}
		out[day] = total
	}
	return out, rows.Err()
}

// DeleteLast cancella il log piu' recente del giorno (undo del quick-log).
// Ritorna ErrWaterLogNotFound se non c'e' nulla da cancellare.
func (w *WaterLogs) DeleteLast(ctx context.Context, userID int64, date time.Time) error {
	res, err := w.db.ExecContext(ctx,
		`DELETE FROM water_logs WHERE id = (
			SELECT id FROM water_logs
			WHERE user_id = ? AND logged_at = ?
			ORDER BY id DESC LIMIT 1
		)`,
		userID, date.Format("2006-01-02"),
	)
	if err != nil {
		return fmt.Errorf("delete last water log: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrWaterLogNotFound
	}
	return nil
}

// listForDate e' usato dai test per verificare i singoli record.
func (w *WaterLogs) listForDate(ctx context.Context, userID int64, date time.Time) ([]domain.WaterLog, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT id, user_id, logged_at, amount_ml, created_at
		 FROM water_logs WHERE user_id = ? AND logged_at = ? ORDER BY id`,
		userID, date.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []domain.WaterLog
	for rows.Next() {
		var l domain.WaterLog
		var dateStr string
		if err := rows.Scan(&l.ID, &l.UserID, &dateStr, &l.AmountML, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.LoggedAt, _ = time.Parse("2006-01-02", dateStr)
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
