// Package recipeimport extracts a recipe from a web page's structured markup.
// It supports schema.org/Recipe in JSON-LD, the format published by the vast
// majority of recipe sites (Giallozafferano, AllRecipes, NYT Cooking, etc.).
//
// The parsing deliberately has no external dependencies: it extracts the
// <script type="application/ld+json"> blocks with a regex and decodes them
// with encoding/json. Parse is pure (it works on []byte) so it can be tested
// without the network; the HTTP fetch lives in fetch.go.
package recipeimport

import (
	"encoding/json"
	"errors"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// ErrNoRecipe is returned when the page holds no schema.org/Recipe block that
// can be parsed.
var ErrNoRecipe = errors.New("nessuna ricetta strutturata trovata nella pagina")

// ParsedRecipe is the result of the extraction. The ingredients are free-text
// lines (e.g. "200 g di farina 00") because schema.org does not structure
// them: the user maps them by hand onto the DB's ingredients.
type ParsedRecipe struct {
	Name        string
	Description string
	Servings    int
	Ingredients []string
}

// ldJSONBlock captures the content of the JSON-LD scripts, tolerant of
// attribute order, single/double quotes and case.
var ldJSONBlock = regexp.MustCompile(`(?is)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)

// Parse extracts the first schema.org/Recipe recipe from the HTML markup.
func Parse(htmlBytes []byte) (*ParsedRecipe, error) {
	matches := ldJSONBlock.FindAllSubmatch(htmlBytes, -1)
	if len(matches) == 0 {
		return nil, ErrNoRecipe
	}

	for _, m := range matches {
		raw := strings.TrimSpace(string(m[1]))
		node := decodeLD(raw)
		if node == nil {
			continue
		}
		if rec := findRecipeNode(node); rec != nil {
			return fromRecipeNode(rec), nil
		}
	}
	return nil, ErrNoRecipe
}

// decodeLD decodes a JSON-LD block that can be an object or an array.
// When the direct parse fails (some sites HTML-escape the content), it
// retries after unescaping the HTML entities.
func decodeLD(raw string) interface{} {
	var node interface{}
	if err := json.Unmarshal([]byte(raw), &node); err == nil {
		return node
	}
	if err := json.Unmarshal([]byte(html.UnescapeString(raw)), &node); err == nil {
		return node
	}
	return nil
}

// findRecipeNode walks the JSON-LD recursively (single object, array, or
// @graph container) and returns the first node whose @type includes "Recipe".
func findRecipeNode(node interface{}) map[string]interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		if typeIncludes(v["@type"], "Recipe") {
			return v
		}
		if g, ok := v["@graph"]; ok {
			if rec := findRecipeNode(g); rec != nil {
				return rec
			}
		}
	case []interface{}:
		for _, item := range v {
			if rec := findRecipeNode(item); rec != nil {
				return rec
			}
		}
	}
	return nil
}

// typeIncludes checks whether the @type field (a string or an array of
// strings) holds the wanted type, case-insensitive.
func typeIncludes(field interface{}, want string) bool {
	switch v := field.(type) {
	case string:
		return strings.EqualFold(v, want)
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.EqualFold(s, want) {
				return true
			}
		}
	}
	return false
}

func fromRecipeNode(rec map[string]interface{}) *ParsedRecipe {
	out := &ParsedRecipe{
		Name:        strings.TrimSpace(firstString(rec["name"])),
		Description: strings.TrimSpace(firstString(rec["description"])),
		Servings:    parseYield(rec["recipeYield"]),
	}
	// recipeIngredient is the standard; some legacy sites use "ingredients".
	ings := stringSlice(rec["recipeIngredient"])
	if len(ings) == 0 {
		ings = stringSlice(rec["ingredients"])
	}
	out.Ingredients = ings
	return out
}

// firstString extracts a string from a field that can be a string or an array.
func firstString(field interface{}) string {
	switch v := field.(type) {
	case string:
		return v
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

// stringSlice normalizes a field into a slice of non-empty strings.
func stringSlice(field interface{}) []string {
	var out []string
	switch v := field.(type) {
	case string:
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

// yieldDigits extracts the first group of digits from a servings string
// (e.g. "4 servings" -> 4, "Per 6 persone" -> 6).
var yieldDigits = regexp.MustCompile(`\d+`)

// parseYield decodes recipeYield, which can be a number, a string
// ("4", "4 servings") or an array. Returns 0 when it cannot be determined.
func parseYield(field interface{}) int {
	switch v := field.(type) {
	case float64:
		return int(v)
	case string:
		if d := yieldDigits.FindString(v); d != "" {
			if n, err := strconv.Atoi(d); err == nil {
				return n
			}
		}
	case []interface{}:
		for _, item := range v {
			if n := parseYield(item); n > 0 {
				return n
			}
		}
	}
	return 0
}
