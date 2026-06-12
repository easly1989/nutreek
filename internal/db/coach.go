package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/amon2126/nutreek/internal/domain"
)

// Errori di dominio della relazione coach <-> cliente.
var (
	ErrCoachClientNotFound = errors.New("cliente non trovato")
	ErrClientHasCoach      = errors.New("il cliente ha gia' un coach")
)

// CoachRepository gestisce la relazione coach -> clienti e l'assegnazione
// dei piani della libreria del coach.
type CoachRepository struct {
	db *sql.DB
}

func NewCoachRepository(db *sql.DB) *CoachRepository {
	return &CoachRepository{db: db}
}

// AddClient collega un cliente al coach. Il vincolo UNIQUE(client_id)
// garantisce al massimo un coach per cliente.
func (c *CoachRepository) AddClient(ctx context.Context, coachID, clientID int64) error {
	_, err := c.db.ExecContext(ctx,
		`INSERT INTO coach_clients (coach_id, client_id) VALUES (?, ?)`,
		coachID, clientID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrClientHasCoach
		}
		return fmt.Errorf("add client: %w", err)
	}
	return nil
}

// RemoveClient scollega il cliente dal coach.
func (c *CoachRepository) RemoveClient(ctx context.Context, coachID, clientID int64) error {
	res, err := c.db.ExecContext(ctx,
		`DELETE FROM coach_clients WHERE coach_id = ? AND client_id = ?`,
		coachID, clientID)
	if err != nil {
		return fmt.Errorf("remove client: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrCoachClientNotFound
	}
	return nil
}

// ListClients restituisce i clienti del coach con il piano assegnato.
func (c *CoachRepository) ListClients(ctx context.Context, coachID int64) ([]domain.CoachClientInfo, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT cc.id, cc.coach_id, cc.client_id, cc.assigned_plan_id, cc.assigned_at,
		        u.id, u.email, u.display_name, u.created_at,
		        COALESCE(np.name, '')
		 FROM coach_clients cc
		 JOIN users u ON u.id = cc.client_id
		 LEFT JOIN nutritionist_plans np ON np.id = cc.assigned_plan_id
		 WHERE cc.coach_id = ?
		 ORDER BY u.display_name, u.id`, coachID)
	if err != nil {
		return nil, fmt.Errorf("list clients: %w", err)
	}
	defer rows.Close()
	var out []domain.CoachClientInfo
	for rows.Next() {
		var info domain.CoachClientInfo
		var client domain.User
		if err := rows.Scan(&info.ID, &info.CoachID, &info.ClientID, &info.AssignedPlanID, &info.AssignedAt,
			&client.ID, &client.Email, &client.DisplayName, &client.CreatedAt,
			&info.AssignedPlanName); err != nil {
			return nil, fmt.Errorf("scan client: %w", err)
		}
		info.Client = &client
		out = append(out, info)
	}
	return out, rows.Err()
}

// AssignPlan assegna al cliente un piano della libreria del coach.
// Verifica in un'unica UPDATE che il piano appartenga al coach e che il
// cliente sia effettivamente suo.
func (c *CoachRepository) AssignPlan(ctx context.Context, coachID, clientID, planID int64) error {
	res, err := c.db.ExecContext(ctx,
		`UPDATE coach_clients
		 SET assigned_plan_id = ?, assigned_at = CURRENT_TIMESTAMP
		 WHERE coach_id = ? AND client_id = ?
		   AND EXISTS (SELECT 1 FROM nutritionist_plans WHERE id = ? AND user_id = ?)`,
		planID, coachID, clientID, planID, coachID)
	if err != nil {
		return fmt.Errorf("assign plan: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrCoachClientNotFound
	}
	return nil
}

// UnassignPlan rimuove l'assegnazione: il cliente torna al proprio piano attivo.
func (c *CoachRepository) UnassignPlan(ctx context.Context, coachID, clientID int64) error {
	res, err := c.db.ExecContext(ctx,
		`UPDATE coach_clients SET assigned_plan_id = NULL
		 WHERE coach_id = ? AND client_id = ?`, coachID, clientID)
	if err != nil {
		return fmt.Errorf("unassign plan: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrCoachClientNotFound
	}
	return nil
}

// GetAssignment restituisce la vista lato cliente (coach + piano assegnato),
// o nil se il cliente non ha un coach.
func (c *CoachRepository) GetAssignment(ctx context.Context, clientID int64) (*domain.CoachAssignment, error) {
	var a domain.CoachAssignment
	err := c.db.QueryRowContext(ctx,
		`SELECT cc.coach_id, u.display_name, cc.assigned_plan_id
		 FROM coach_clients cc
		 JOIN users u ON u.id = cc.coach_id
		 WHERE cc.client_id = ?`, clientID).
		Scan(&a.CoachID, &a.CoachName, &a.AssignedPlanID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get assignment: %w", err)
	}
	return &a, nil
}
