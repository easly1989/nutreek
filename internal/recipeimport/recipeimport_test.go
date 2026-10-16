package recipeimport

import (
	"context"
	"errors"
	"testing"
)

func testCtx() context.Context { return context.Background() }

func TestParseSingleRecipeObject(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	{
		"@context": "https://schema.org",
		"@type": "Recipe",
		"name": "Pasta al pomodoro",
		"description": "Un classico veloce.",
		"recipeYield": "4 porzioni",
		"recipeIngredient": ["320 g di pasta", "400 g di pomodori pelati", "Olio EVO q.b."]
	}
	</script></head><body></body></html>`

	rec, err := Parse([]byte(html))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rec.Name != "Pasta al pomodoro" {
		t.Errorf("name: %q", rec.Name)
	}
	if rec.Servings != 4 {
		t.Errorf("servings: %d", rec.Servings)
	}
	if len(rec.Ingredients) != 3 || rec.Ingredients[0] != "320 g di pasta" {
		t.Errorf("ingredients: %v", rec.Ingredients)
	}
}

func TestParseGraphArray(t *testing.T) {
	// Many sites (WordPress + Yoast) use @graph with several nodes.
	html := `<script type="application/ld+json">
	{"@context":"https://schema.org","@graph":[
		{"@type":"WebSite","name":"Blog"},
		{"@type":["Recipe","Thing"],"name":"Torta di mele","recipeYield":6,
		 "recipeIngredient":["3 mele","200 g farina"]}
	]}</script>`

	rec, err := Parse([]byte(html))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rec.Name != "Torta di mele" {
		t.Errorf("name: %q", rec.Name)
	}
	if rec.Servings != 6 {
		t.Errorf("servings: %d", rec.Servings)
	}
	if len(rec.Ingredients) != 2 {
		t.Errorf("ingredients: %v", rec.Ingredients)
	}
}

func TestParseTopLevelArray(t *testing.T) {
	html := `<script type="application/ld+json">
	[{"@type":"BreadcrumbList"},{"@type":"Recipe","name":"Risotto","recipeYield":["2","2 servings"],
	  "recipeIngredient":"320 g riso"}]</script>`

	rec, err := Parse([]byte(html))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rec.Name != "Risotto" {
		t.Errorf("name: %q", rec.Name)
	}
	if rec.Servings != 2 {
		t.Errorf("servings: %d", rec.Servings)
	}
	// A single-string recipeIngredient -> one line.
	if len(rec.Ingredients) != 1 || rec.Ingredients[0] != "320 g riso" {
		t.Errorf("ingredients: %v", rec.Ingredients)
	}
}

func TestParseHTMLEscapedJSON(t *testing.T) {
	// Some sites escape the quotes in the script block.
	html := `<script type='application/ld+json'>
	{&quot;@type&quot;:&quot;Recipe&quot;,&quot;name&quot;:&quot;Insalata&quot;,&quot;recipeIngredient&quot;:[&quot;lattuga&quot;]}
	</script>`

	rec, err := Parse([]byte(html))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rec.Name != "Insalata" {
		t.Errorf("name: %q", rec.Name)
	}
}

func TestParseNoRecipe(t *testing.T) {
	html := `<html><body><p>nessun markup</p></body></html>`
	_, err := Parse([]byte(html))
	if !errors.Is(err, ErrNoRecipe) {
		t.Fatalf("want ErrNoRecipe, got %v", err)
	}

	// JSON-LD present but it is not a recipe.
	html2 := `<script type="application/ld+json">{"@type":"Article","name":"x"}</script>`
	if _, err := Parse([]byte(html2)); !errors.Is(err, ErrNoRecipe) {
		t.Fatalf("want ErrNoRecipe for Article, got %v", err)
	}
}

func TestValidatePublicURLScheme(t *testing.T) {
	for _, raw := range []string{"ftp://example.com", "file:///etc/passwd", "javascript:alert(1)"} {
		f := NewFetcher()
		_, err := f.FetchAndParse(testCtx(), raw)
		if !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%q: want ErrInvalidURL, got %v", raw, err)
		}
	}
}

func TestValidatePublicURLBlocksLoopback(t *testing.T) {
	f := NewFetcher()
	// 127.0.0.1 and localhost must be blocked (SSRF).
	for _, raw := range []string{"http://127.0.0.1/admin", "http://localhost:8080/"} {
		_, err := f.FetchAndParse(testCtx(), raw)
		if !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%q: want ErrInvalidURL, got %v", raw, err)
		}
	}
}
