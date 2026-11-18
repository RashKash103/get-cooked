package importer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const page = `<!doctype html><html><head>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"WebSite","name":"x"}</script>
<script type="application/ld+json">
{"@context":"https://schema.org","@graph":[
 {"@type":"WebPage","name":"page"},
 {"@type":["Recipe"],"name":"Lemon &amp; Herb Chicken","description":"<p>Bright and easy.</p>",
  "image":[{"@type":"ImageObject","url":"https://example.com/c.jpg"}],
  "recipeYield":["4","4 servings"],"prepTime":"PT15M","cookTime":"PT1H5M","totalTime":"PT1H20M",
  "recipeCategory":"Dinner","recipeCuisine":"Greek","keywords":"chicken, Lemon, weeknight",
  "author":{"@type":"Person","name":"Alex"},
  "recipeIngredient":["2 lb chicken thighs","1 lemon, juiced","2 tbsp olive oil"],
  "recipeInstructions":[
    {"@type":"HowToSection","name":"Marinate","itemListElement":[{"@type":"HowToStep","text":"Mix the lemon and oil."}]},
    {"@type":"HowToSection","name":"Cook","itemListElement":[{"@type":"HowToStep","text":"Roast at 425&deg;F for 40 minutes."}]}
  ]}
]}
</script></head><body></body></html>`

func TestExtract(t *testing.T) {
	r, err := Extract([]byte(page))
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Lemon & Herb Chicken" || r.Description != "Bright and easy." || r.Image != "https://example.com/c.jpg" {
		t.Errorf("meta: %+v", r)
	}
	if r.Yield != 4 || r.Prep != 15 || r.Cook != 65 || r.Total != 80 || r.Author != "Alex" {
		t.Errorf("numbers: %+v", r)
	}
	if len(r.Ingredients) != 3 || len(r.Instructions) != 2 || r.Instructions[1].Steps[0] != "Roast at 425°F for 40 minutes." {
		t.Errorf("lists: %+v", r.Instructions)
	}
	y, err := r.YAML("https://example.com/r")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"title: Lemon & Herb Chicken", "section: Marinate", "- 2 lb chicken thighs", "prep: 15", "cook: 65", "url: https://example.com/r"} {
		if !strings.Contains(string(y), want) {
			t.Errorf("YAML missing %q:\n%s", want, y)
		}
	}
	if strings.Contains(string(y), "total:") {
		t.Error("total equal to prep+cook should be omitted")
	}
}

func TestNoRecipe(t *testing.T) {
	if _, err := Extract([]byte("<html></html>")); err != ErrNoRecipe {
		t.Fatalf("err = %v", err)
	}
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()
	b, err := Fetch(context.Background(), srv.URL+"/r")
	if err != nil || !strings.Contains(string(b), "Lemon") {
		t.Fatalf("fetch: %v", err)
	}
	if _, err := Fetch(context.Background(), srv.URL+"/missing"); err == nil {
		t.Fatal("expected 404 error")
	}
}

func TestMinutes(t *testing.T) {
	for in, want := range map[string]float64{"PT30M": 30, "PT1H": 60, "P1DT2H": 1560, "PT90S": 2, "bogus": 0, "pt5m": 5} {
		if got := Minutes(in); got != want {
			t.Errorf("Minutes(%q) = %v want %v", in, got, want)
		}
	}
}
