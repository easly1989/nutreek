package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/db"
)

// parseID reads a URL path parameter as int64.
func parseID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, key), 10, 64)
}

// parseIDList reads the `ids` form value as a comma-separated list and
// returns the matching int64s. Error when empty or malformed.
func parseIDList(s string) ([]int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("ids vuoto")
	}
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("id non valido: %q", p)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ids vuoto dopo parsing")
	}
	return out, nil
}

// parseWeekStart reads ?start=YYYY-MM-DD and normalizes it to Monday. When
// missing or invalid, it returns the Monday of the current week.
func parseWeekStart(r *http.Request) time.Time {
	q := strings.TrimSpace(r.URL.Query().Get("start"))
	if q != "" {
		if t, err := time.Parse("2006-01-02", q); err == nil {
			return db.MondayOf(t)
		}
	}
	return db.MondayOf(time.Now())
}
