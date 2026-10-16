package domain

// Feature is an app feature the owner can manage.
// IsPremium says whether it needs an entitlement when gating is on.
// Enabled is the global kill switch: when false the feature is off for everyone.
type Feature struct {
	Key         string
	Name        string
	Description string
	IsPremium   bool
	Enabled     bool
	SortOrder   int
}
