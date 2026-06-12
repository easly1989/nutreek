package domain

import "time"

// CoachClient rappresenta la relazione coach -> cliente.
// AssignedPlanID e' il piano della libreria del coach assegnato al cliente
// (nil = nessun piano assegnato, il cliente usa il proprio piano attivo).
type CoachClient struct {
	ID             int64
	CoachID        int64
	ClientID       int64
	AssignedPlanID *int64
	AssignedAt     time.Time
}

// CoachClientInfo aggrega la relazione con i dati del cliente e del piano
// assegnato, per la dashboard coach.
type CoachClientInfo struct {
	CoachClient
	Client           *User
	AssignedPlanName string // vuoto se nessun piano assegnato
}

// CoachAssignment e' la vista lato cliente: chi e' il mio coach e quale
// piano mi ha assegnato.
type CoachAssignment struct {
	CoachID        int64
	CoachName      string
	AssignedPlanID *int64
}
