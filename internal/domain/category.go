package domain

type IngredientCategory struct {
	ID        int64
	UserID    int64
	Name      string
	SortOrder int
}

type AllergenTag struct {
	ID   int64
	Code string // stable key for translation, empty for rows not seeded by the migrations
	Name string
}
