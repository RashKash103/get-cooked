package site

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RashKash103/get-cooked/internal/assets"
	"github.com/RashKash103/get-cooked/internal/runtime"
)

var update = flag.Bool("update", false, "update golden files")

const exampleSite = "../../examples/site"

func loadExample(t *testing.T) *Site {
	t.Helper()
	s, err := Load(exampleSite, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Diags.HasErrors() {
		t.Fatal(s.Diags)
	}
	return s
}

// TestBlocksGolden writes the HTML of every dynamic block for a set of
// recipes, languages and states. web/src/ui/blocks.test.ts renders the same
// states with the TypeScript runtime + blocks and must produce identical HTML.
func TestBlocksGolden(t *testing.T) {
	s := loadExample(t)
	type tc struct {
		Lang     string            `json:"lang"`
		Slug     string            `json:"slug"`
		Base     string            `json:"base"`
		Dict     map[string]string `json:"dict"`
		Compiled any               `json:"compiled"`
		State    runtime.State     `json:"state"`
		HTML     map[string]string `json:"html"`
	}
	states := []runtime.State{
		{},
		{Servings: 1, System: "metric"},
		{Servings: 9, System: "us", Options: map[string]any{"cranberries": false, "walnuts": true, "sweetener": "brown_sugar", "method": "air_fryer", "heat": 3.0, "wheat_bread": true}},
	}
	var cases []tc
	for _, l := range s.Langs {
		dict := map[string]string{}
		for _, k := range jsKeys {
			dict[k] = l.Catalog.T(k)
		}
		base := "/" + l.Prefix(s.Langs[0].Code)
		for _, r := range l.Recipes {
			if l.Code != s.Langs[0].Code && !l.Translated[r.Slug] && r.Slug != "crispy-chicken-thighs" {
				continue
			}
			for _, st := range states {
				m, err := runtime.Render(r.C, st)
				if err != nil {
					t.Fatalf("%s %s: %v", l.Code, r.Slug, err)
				}
				cases = append(cases, tc{
					Lang: l.Code, Slug: r.Slug, Base: base, Dict: dict, Compiled: r.C, State: st,
					HTML: map[string]string{
						"times":       renderTimes(m),
						"equipment":   renderEquipment(m),
						"ingredients": renderIngredients(m, base, l.Catalog),
						"steps":       renderSteps(m, base, l.Catalog),
						"warnings":    renderNotes(m, true),
						"tips":        renderNotes(m, false),
					},
				})
			}
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(cases); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "testdata", "conformance", "blocks", "blocks.json")
	if *update {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(want, buf.Bytes()) {
		t.Errorf("%s out of date; run: go test ./internal/site -update", path)
	}
}

func TestLanguages(t *testing.T) {
	s := loadExample(t)
	if len(s.Langs) != 2 || s.Langs[1].Code != "es" {
		t.Fatalf("langs = %v", s.Config.Languages)
	}
	es := s.Langs[1]
	if !es.Translated["cranberry-baked-oatmeal"] || es.Translated["crispy-chicken-thighs"] {
		t.Errorf("translated = %v", es.Translated)
	}
	var oat, chicken *runtime.Model
	for _, r := range es.Recipes {
		m, _ := runtime.Render(r.C, runtime.State{})
		switch r.Slug {
		case "cranberry-baked-oatmeal":
			oat = m
			if r.Title != "Avena horneada con arándanos" {
				t.Errorf("title = %q", r.Title)
			}
		case "crispy-chicken-thighs":
			chicken = m
			if r.Title != "Crispy Smoked-Paprika Chicken Thighs" {
				t.Errorf("untranslated recipe should keep its title, got %q", r.Title)
			}
		}
	}
	if oat == nil || oat.System != "metric" || oat.Yield != "4 porciones" {
		t.Fatalf("oat = %+v", oat)
	}
	first := oat.Ingredients[0].Items[0]
	if first.Q.String()+first.J+first.Name != "180 g de copos de avena" {
		t.Errorf("first ingredient = %q", first.Q.String()+first.J+first.Name)
	}
	// Untranslated recipes still get translated units and ingredient names.
	if chicken.Times[0].Label != "Preparación" || !strings.Contains(chicken.Ingredients[0].Items[0].Name, "muslos de pollo") {
		t.Errorf("chicken = %+v %+v", chicken.Times[0], chicken.Ingredients[0].Items[0])
	}
}

func TestBuildMultilingual(t *testing.T) {
	if !assets.Built() {
		t.Skip("frontend bundle not built")
	}
	s := loadExample(t)
	out := t.TempDir()
	if err := Build(s, BuildOptions{Out: out, BaseURL: "https://example.com/kitchen/"}); err != nil {
		t.Fatal(err)
	}
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(out, p))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	en := read("recipes/cranberry-baked-oatmeal/index.html")
	es := read("es/recipes/cranberry-baked-oatmeal/index.html")
	esChicken := read("es/recipes/crispy-chicken-thighs/index.html")
	checks := []struct {
		page, want string
	}{
		{en, `<html lang="en"`},
		{en, `hreflang="es" href="/kitchen/es/recipes/cranberry-baked-oatmeal/"`},
		{en, `id="lang-picker"`},
		{en, `href="/kitchen/assets/app.`},
		{es, `<html lang="es"`},
		{es, `href="/kitchen/assets/app.`},
		{es, "Avena horneada con arándanos"},
		{es, "Ingredientes"},
		{es, "de copos de avena"},
		{es, `"inLanguage":"es"`},
		{esChicken, `lang="en"`},
		{esChicken, "Esta receta aún no está traducida."},
		{read("es/index.html"), "Buscar recetas o ingredientes"},
		{read("sitemap.xml"), "https://example.com/kitchen/es/recipes/cranberry-baked-oatmeal/"},
		{read("sw.js"), `self.__GC={"base":"/kitchen/","precache":["/kitchen/assets/app.`},
	}
	for _, c := range checks {
		if !strings.Contains(c.page, c.want) {
			t.Errorf("missing %q", c.want)
		}
	}
}
