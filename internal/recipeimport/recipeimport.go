// Package recipeimport estrae una ricetta dal markup strutturato di una
// pagina web. Supporta schema.org/Recipe in formato JSON-LD, il formato
// pubblicato dalla stragrande maggioranza dei siti di ricette (Giallozafferano,
// AllRecipes, NYT Cooking, ecc.).
//
// Il parsing e' volutamente senza dipendenze esterne: estrae i blocchi
// <script type="application/ld+json"> con una regex e li interpreta con
// encoding/json. La funzione Parse e' pura (opera su []byte) cosi' da essere
// testabile senza rete; il fetch HTTP vive in fetch.go.
package recipeimport

import (
	"encoding/json"
	"errors"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// ErrNoRecipe viene restituito quando la pagina non contiene un blocco
// schema.org/Recipe interpretabile.
var ErrNoRecipe = errors.New("nessuna ricetta strutturata trovata nella pagina")

// ParsedRecipe e' il risultato dell'estrazione. Gli ingredienti sono righe di
// testo libero (es. "200 g di farina 00") perche' schema.org non li struttura:
// l'utente li mappa manualmente sugli ingredienti del DB.
type ParsedRecipe struct {
	Name        string
	Description string
	Servings    int
	Ingredients []string
}

// ldJSONBlock cattura il contenuto degli script JSON-LD, tollerante a ordine
// degli attributi, virgolette singole/doppie e maiuscole.
var ldJSONBlock = regexp.MustCompile(`(?is)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)

// Parse estrae la prima ricetta schema.org/Recipe dal markup HTML.
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

// decodeLD interpreta un blocco JSON-LD che puo' essere un oggetto o un array.
// Se il parse diretto fallisce (alcuni siti HTML-escapano il contenuto), ritenta
// dopo aver de-escapato le entita' HTML.
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

// findRecipeNode percorre ricorsivamente il JSON-LD (oggetto singolo, array, o
// contenitore @graph) e ritorna il primo nodo il cui @type include "Recipe".
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

// typeIncludes verifica se il campo @type (stringa o array di stringhe)
// contiene il tipo cercato, case-insensitive.
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
	// recipeIngredient e' lo standard; alcuni siti legacy usano "ingredients".
	ings := stringSlice(rec["recipeIngredient"])
	if len(ings) == 0 {
		ings = stringSlice(rec["ingredients"])
	}
	out.Ingredients = ings
	return out
}

// firstString estrae una stringa da un campo che puo' essere string o array.
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

// stringSlice normalizza un campo in una slice di stringhe non vuote.
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

// yieldDigits estrae il primo gruppo di cifre da una stringa di porzioni
// (es. "4 servings" -> 4, "Per 6 persone" -> 6).
var yieldDigits = regexp.MustCompile(`\d+`)

// parseYield interpreta recipeYield, che puo' essere numero, stringa
// ("4", "4 servings") o array. Ritorna 0 se non determinabile.
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
