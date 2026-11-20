package site

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RashKash103/get-cooked/internal/assets"
	"github.com/RashKash103/get-cooked/internal/export"
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
		Meta     map[string]any    `json:"meta"`
		HTML     map[string]string `json:"html"`
	}
	states := []runtime.State{
		{},
		{Servings: 1, System: "metric"},
		{Servings: 9, System: "us", Options: map[string]any{"cranberries": false, "walnuts": true, "sweetener": "brown_sugar", "method": "air_fryer", "heat": 3.0, "wheat_bread": true}},
	}
	var cases []tc
	// A synthetic language with Arabic-Indic digits, ASCII fractions and an
	// Arabic decimal separator, derived from Spanish, so localized digits are
	// covered by the Go↔TS parity check.
	es := s.Langs[1]
	arCat := *es.Catalog
	arCat.Lang, arCat.Digits, arCat.Fractions, arCat.Decimal = "ar-test", "٠١٢٣٤٥٦٧٨٩", "native", "٫"
	arCat.FracGlyphs = map[string]string{"1/4": "¼"}
	arCat.FracDisplay = "diagonal"
	ar := &Lang{Code: "ar-test", Catalog: &arCat, Units: arCat.Localize(s.Units), Translated: es.Translated}
	for _, r := range es.Recipes {
		if r.Slug != "cranberry-baked-oatmeal" && r.Slug != "crispy-chicken-thighs" {
			continue
		}
		rc := *r
		c := *r.C
		c.Units = ar.Units
		rc.C = &c
		ar.Recipes = append(ar.Recipes, &rc)
	}
	langs := append(append([]*Lang{}, s.Langs...), ar)
	for _, l := range langs {
		dict := map[string]string{}
		for _, k := range jsKeys {
			dict[k] = l.Catalog.T(k)
		}
		dict["_digits"] = l.Catalog.Digits
		dict["_fracs"] = l.Catalog.FracDisplay
		fr := func(h string) string { return h }
		if l.Catalog.Diagonal() {
			fr = diagonalFractions
		}
		base := "/" + l.Prefix(s.Langs[0].Code)
		for _, r := range l.Recipes {
			if l.Code != s.Langs[0].Code && l.Code != "ar-test" && !l.Translated[r.Slug] && r.Slug != "crispy-chicken-thighs" {
				continue
			}
			for _, st := range states {
				m, err := runtime.Render(r.C, st)
				if err != nil {
					t.Fatalf("%s %s: %v", l.Code, r.Slug, err)
				}
				opts := map[string]any{}
				for _, o := range r.C.Options {
					opts[o.ID] = o.Default
				}
				for k, v := range st.Options {
					opts[k] = v
				}
				md := export.Markdown(export.Doc{R: r, M: m, Cat: l.Catalog, Opts: opts})
				cases = append(cases, tc{
					Lang: l.Code, Slug: r.Slug, Base: base, Dict: dict, Compiled: r.C, State: st,
					Meta: map[string]any{"title": r.Title, "description": r.Description, "source": sourceJSON(r.Source)},
					HTML: map[string]string{
						"times":       fr(renderTimes(m, l.Catalog)),
						"equipment":   fr(renderEquipment(m)),
						"ingredients": fr(renderIngredients(m, base, l.Catalog)),
						"steps":       fr(renderSteps(m, base, l.Catalog)),
						"warnings":    fr(renderNotes(m, true)),
						"tips":        fr(renderNotes(m, false)),
						"markdown":    md,
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
		{read("recipes/crispy-chicken-thighs/index.html"), `"src":"/kitchen/recipes/crispy-chicken-thighs/crispy-chicken-thighs-air-fryer.svg"`},
		{read("recipes/crispy-chicken-thighs/index.html"), `data-default-src="/kitchen/recipes/crispy-chicken-thighs/crispy-chicken-thighs.svg"`},
		{read("es/recipes/crispy-chicken-thighs/crispy-chicken-thighs-air-fryer.svg"), "<svg"},
		{read("index.html"), `src="/kitchen/recipes/buttermilk-pancakes/buttermilk-pancakes.svg"`},
		{read("sw.js"), `self.__GC={"base":"/kitchen/","offline":"/kitchen/offline.html","precache":["/kitchen/assets/app.`},
	}
	for _, c := range checks {
		if !strings.Contains(c.page, c.want) {
			t.Errorf("missing %q", c.want)
		}
	}
}

func TestPWA(t *testing.T) {
	if !assets.Built() {
		t.Skip("frontend bundle not built")
	}
	s := loadExample(t)
	out := t.TempDir()
	if err := Build(s, BuildOptions{Out: out, BaseURL: "/kitchen/"}); err != nil {
		t.Fatal(err)
	}
	var m struct {
		ID, Name, StartURL, Scope, Display, ThemeColor, Lang string
		Icons                                                []struct{ Src, Sizes, Type, Purpose string }
	}
	b, err := os.ReadFile(filepath.Join(out, "manifest.webmanifest"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	m.ID, _ = raw["id"].(string)
	m.StartURL, _ = raw["start_url"].(string)
	m.Display, _ = raw["display"].(string)
	m.ThemeColor, _ = raw["theme_color"].(string)
	m.Lang, _ = raw["lang"].(string)
	if m.ID != "/kitchen/" || m.StartURL != "/kitchen/" || m.Display != "standalone" || m.ThemeColor != s.Config.Theme.Accent || m.Lang != "en" {
		t.Errorf("manifest = %s", b)
	}
	icons, _ := raw["icons"].([]any)
	want := map[string]bool{"192x192 any": false, "512x512 any": false, "512x512 maskable": false}
	for _, i := range icons {
		ic := i.(map[string]any)
		key := fmt.Sprintf("%v %v", ic["sizes"], ic["purpose"])
		if _, ok := want[key]; ok {
			want[key] = true
			src := strings.TrimPrefix(ic["src"].(string), "/kitchen/")
			f, err := os.Open(filepath.Join(out, src))
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := png.DecodeConfig(f)
			_ = f.Close()
			if err != nil || fmt.Sprintf("%dx%d", cfg.Width, cfg.Height) != ic["sizes"] {
				t.Errorf("%s: %v %+v", src, err, cfg)
			}
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("manifest missing %s icon", k)
		}
	}
	for _, f := range []string{"apple-touch-icon.png", "offline.html"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	sw, _ := os.ReadFile(filepath.Join(out, "sw.js"))
	if !strings.Contains(string(sw), `"offline":"/kitchen/offline.html"`) {
		t.Error("service worker should know the offline page")
	}

	// A custom icon is resized to every size.
	src := image.NewRGBA(image.Rect(0, 0, 600, 600))
	draw.Draw(src, src.Bounds(), &image.Uniform{C: color.RGBA{0x10, 0x80, 0x40, 0xff}}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	_ = png.Encode(&buf, src)
	dir := t.TempDir()
	if err := copyDir(exampleSite, dir); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "my-icon.png"), buf.Bytes(), 0o644)
	cfgFile := filepath.Join(dir, "get-cooked.yaml")
	cb, _ := os.ReadFile(cfgFile)
	_ = os.WriteFile(cfgFile, append(cb, []byte("icon: my-icon.png\n")...), 0o644)
	s2, err := Load(dir, LoadOptions{})
	if err != nil || s2.Diags.HasErrors() {
		t.Fatal(err, s2.Diags)
	}
	out2 := t.TempDir()
	if err := Build(s2, BuildOptions{Out: out2}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(filepath.Join(out2, "icon-maskable-512.png"))
	img, err := png.Decode(f)
	_ = f.Close()
	if err != nil || img.Bounds().Dx() != 512 {
		t.Fatalf("custom maskable icon: %v", err)
	}
	if r, g, _, _ := img.At(2, 2).RGBA(); r>>8 != 0x10 || g>>8 != 0x80 {
		t.Error("maskable background should use the icon's corner colour")
	}
	if _, err := os.Stat(filepath.Join(out2, "icon.svg")); err == nil {
		t.Error("custom icon: the default SVG icon should not be written")
	}
}

func TestGenerator(t *testing.T) {
	for v, want := range map[string]generator{
		"0.6.0":                               {"v0.6.0", "https://github.com/RashKash103/get-cooked/releases/tag/v0.6.0"},
		"v0.6.0":                              {"v0.6.0", "https://github.com/RashKash103/get-cooked/releases/tag/v0.6.0"},
		"dev":                                 {"dev", "https://github.com/RashKash103/get-cooked/releases"},
		"":                                    {"dev", "https://github.com/RashKash103/get-cooked/releases"},
		"0.6.1-0.20261119162739-9e2571032bcd": {"0.6.1-0.20261119162739-9e2571032bcd", "https://github.com/RashKash103/get-cooked/releases"},
	} {
		if got := newGenerator(v); got != want {
			t.Errorf("newGenerator(%q) = %+v want %+v", v, got, want)
		}
	}
}
