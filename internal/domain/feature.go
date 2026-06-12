package domain

// Feature rappresenta una funzionalità dell'app gestibile dall'owner.
// IsPremium indica se richiede un entitlement quando il gating è attivo.
// Enabled è il kill-switch globale: se false la feature è disattivata per tutti.
type Feature struct {
	Key         string
	Name        string
	Description string
	IsPremium   bool
	Enabled     bool
	SortOrder   int
}
