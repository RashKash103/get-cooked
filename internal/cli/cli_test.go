package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RashKash103/get-cooked/internal/assets"
	"github.com/RashKash103/get-cooked/internal/scaffold"
	"github.com/RashKash103/get-cooked/internal/skills"
)

const exampleSite = "../../examples/site"

func run(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Execute(args, &out, &errb)
	return out.String() + errb.String(), code
}

func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, code := run(t, args...)
	if code != 0 {
		t.Fatalf("get-cooked %s failed (%d):\n%s", strings.Join(args, " "), code, out)
	}
	return out
}

func TestVersionDocsSchema(t *testing.T) {
	if out := mustRun(t, "version"); !strings.Contains(out, "get-cooked") {
		t.Error(out)
	}
	if out := mustRun(t, "docs"); !strings.Contains(out, "# get-cooked recipe format") {
		t.Error("docs missing")
	}
	for _, s := range []string{"recipe", "ingredients", "config", "i18n"} {
		if out := mustRun(t, "schema", s); !strings.Contains(out, `"$schema"`) {
			t.Errorf("schema %s", s)
		}
	}
	if _, code := run(t, "schema", "nope"); code == 0 {
		t.Error("unknown schema should fail")
	}
}

func TestValidateExample(t *testing.T) {
	out := mustRun(t, "validate", "-s", exampleSite)
	if !strings.Contains(out, "recipes OK") {
		t.Error(out)
	}
	out = mustRun(t, "validate", "-s", exampleSite, "--format", "json")
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Error(out)
	}
}

func TestRender(t *testing.T) {
	r := filepath.Join(exampleSite, "recipes/breakfast/cranberry-baked-oatmeal.yaml")
	out := mustRun(t, "render", r, "--servings", "1", "--set", "cranberries=false")
	if !strings.Contains(out, "8×8 inch baking dish") || !strings.Contains(out, "softer") || strings.Contains(out, "cranberries") {
		t.Errorf("render:\n%s", out)
	}
	out = mustRun(t, "render", r, "--lang", "es")
	if !strings.Contains(out, "Avena horneada") || !strings.Contains(out, "de copos de avena") {
		t.Errorf("render es:\n%s", out)
	}
	if out := mustRun(t, "render", r, "--json"); !strings.Contains(out, `"yield": "4 servings"`) {
		t.Errorf("json:\n%s", out)
	}
	if _, code := run(t, "render", r, "--lang", "xx"); code == 0 {
		t.Error("unknown language should fail")
	}
}

func TestExport(t *testing.T) {
	r := filepath.Join(exampleSite, "recipes/breakfast/buttermilk-pancakes.yaml")
	out := mustRun(t, "export", r, "--servings", "2", "--set", "no_buttermilk=true")
	if !strings.Contains(out, "# Fluffy Buttermilk Pancakes") || !strings.Contains(out, "*I don't have buttermilk*") {
		t.Errorf("md:\n%s", out)
	}
	if out := mustRun(t, "export", r, "-f", "typ", "--lang", "es"); !strings.Contains(out, `#section("Ingredientes")`) {
		t.Errorf("typ:\n%s", out)
	}
	dst := filepath.Join(t.TempDir(), "p.txt")
	mustRun(t, "export", r, "-o", dst)
	if b, _ := os.ReadFile(dst); strings.Contains(string(b), "**") {
		t.Error("txt should have no markup")
	}
	if _, code := run(t, "export", r, "-f", "docx"); code == 0 {
		t.Error("unknown format should fail")
	}
}

func TestMigrateCommand(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Write(dir, "T"); err != nil {
		t.Fatal(err)
	}
	if out := mustRun(t, "migrate", "-s", dir); !strings.Contains(out, "up to date") {
		t.Errorf("fresh site should be up to date:\n%s", out)
	}
	db := filepath.Join(dir, "ingredients.yaml")
	b, _ := os.ReadFile(db)
	old := strings.Replace(string(b), "density: { value: 125, unit: g/cup }", "grams_per_cup: 125", 1)
	if err := os.WriteFile(db, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, "validate", "-s", dir)
	if code == 0 || !strings.Contains(out, "get-cooked migrate") {
		t.Errorf("validate should point at migrate (%d):\n%s", code, out)
	}
	if out := mustRun(t, "migrate", "-s", dir, "--dry-run"); !strings.Contains(out, "would migrate") || !strings.Contains(out, "ingredients.yaml") {
		t.Errorf("dry run:\n%s", out)
	}
	mustRun(t, "migrate", "-s", dir)
	mustRun(t, "validate", "-s", dir)
	if out := mustRun(t, "migrate", "--list"); !strings.Contains(out, "0.3.0-density") {
		t.Error(out)
	}
}

func TestStrict(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Write(dir, "T"); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "recipes/x.yaml")
	_ = os.WriteFile(bad, []byte("title: X\nservings: 2\ningredients: [{id: dust, name: moon dust, amount: 1, unit: cup}]\nsteps: [Add @dust.]\n"), 0o644)
	if out := mustRun(t, "validate", "-s", dir); strings.Contains(out, "moon dust") {
		t.Error("non-strict should not warn")
	}
	if out := mustRun(t, "validate", "-s", dir, "--strict"); !strings.Contains(out, `"moon dust" has no density`) {
		t.Errorf("strict:\n%s", out)
	}
}

func TestInitNewBuildAndErrors(t *testing.T) {
	if !assets.Built() {
		t.Skip("frontend bundle not built (run pnpm build)")
	}
	dir := t.TempDir()
	out := mustRun(t, "init", dir, "--title", "Test Recipes")
	if !strings.Contains(out, "get-cooked.yaml") {
		t.Error(out)
	}
	if out := mustRun(t, "init", dir); !strings.Contains(out, "nothing to do") {
		t.Error("second init should not overwrite")
	}
	mustRun(t, "new", "Lemon Bars", "-s", dir, "-c", "Desserts")
	if _, err := os.Stat(filepath.Join(dir, "recipes/desserts/lemon-bars.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, code := run(t, "new", "Lemon Bars", "-s", dir, "-c", "Desserts"); code == 0 {
		t.Error("duplicate new should fail")
	}
	mustRun(t, "validate", "-s", dir)
	pub := filepath.Join(dir, "out")
	mustRun(t, "build", "-s", dir, "-o", pub, "--base-url", "https://example.com/r/")
	for _, f := range []string{"index.html", "404.html", "sw.js", "manifest.webmanifest", "sitemap.xml", "robots.txt", "feed.xml", "list/index.html",
		"recipes/lemon-bars/index.html", "recipes/cranberry-baked-oatmeal/index.html", "recipes/cranberry-baked-oatmeal/cranberry-baked-oatmeal.svg"} {
		if _, err := os.Stat(filepath.Join(pub, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	page, _ := os.ReadFile(filepath.Join(pub, "recipes/cranberry-baked-oatmeal/index.html"))
	for _, want := range []string{`"@type":"Recipe"`, `href="/r/assets/app.`, `<link rel="canonical" href="https://example.com/r/recipes/cranberry-baked-oatmeal/">`, `id="recipe-data"`} {
		if !bytes.Contains(page, []byte(want)) {
			t.Errorf("page missing %q", want)
		}
	}
	sitemap, _ := os.ReadFile(filepath.Join(pub, "sitemap.xml"))
	if !bytes.Contains(sitemap, []byte("https://example.com/r/recipes/lemon-bars/")) {
		t.Errorf("sitemap:\n%s", sitemap)
	}

	// A broken recipe fails validation with a file:line diagnostic and
	// GitHub annotations.
	bad := filepath.Join(dir, "recipes/bad.yaml")
	if err := os.WriteFile(bad, []byte("title: Bad\nservings: 2\ningredients: [salt]\nsteps:\n  - Add @slat and bake {servings * }.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, "validate", "-s", dir, "--format", "github")
	if code == 0 || !strings.Contains(out, "::error file=") || !strings.Contains(out, "bad.yaml,line=5") {
		t.Errorf("validate bad (%d):\n%s", code, out)
	}
	if _, code := run(t, "build", "-s", dir, "-o", pub); code == 0 {
		t.Error("build should fail with errors")
	}
}

func TestI18nWorkflow(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Write(dir, "T"); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "get-cooked.yaml")
	b, _ := os.ReadFile(cfg)
	if err := os.WriteFile(cfg, bytes.Replace(b, []byte("languages: [en]"), []byte("languages: [en, fr]"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "i18n", "init", "fr", "-s", dir)
	if _, code := run(t, "i18n", "init", "fr", "-s", dir); code == 0 {
		t.Error("init twice should fail without --force")
	}
	out := mustRun(t, "i18n", "extract", "-s", dir)
	if !strings.Contains(out, "cranberry-baked-oatmeal.fr.yaml") || !strings.Contains(out, "to translate") {
		t.Error(out)
	}
	tr := filepath.Join(dir, "recipes/breakfast/cranberry-baked-oatmeal.fr.yaml")
	b, _ = os.ReadFile(tr)
	b = bytes.Replace(b, []byte(`Cranberry Baked Oatmeal: ""`), []byte(`Cranberry Baked Oatmeal: "Flocons d'avoine aux canneberges"`), 1)
	// A broken reference in a translation is reported.
	b = bytes.Replace(b, []byte(`"Let it rest ~{5} before serving warm.": ""`), []byte(`"Let it rest ~{5} before serving warm.": "Laisser reposer @nope."`), 1)
	if err := os.WriteFile(tr, b, 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, "validate", "-s", dir)
	if code == 0 || !strings.Contains(out, "[fr translation]") || !strings.Contains(out, "@nope") {
		t.Errorf("validate translation (%d):\n%s", code, out)
	}
	b = bytes.Replace(b, []byte("Laisser reposer @nope."), []byte("Laisser reposer ~{5}."), 1)
	_ = os.WriteFile(tr, b, 0o644)
	out = mustRun(t, "validate", "-s", dir)
	if !strings.Contains(out, "untranslated") {
		t.Errorf("expected untranslated warning:\n%s", out)
	}
	if out := mustRun(t, "render", filepath.Join(dir, "recipes/breakfast/cranberry-baked-oatmeal.yaml"), "-s", dir, "--lang", "fr"); !strings.Contains(out, "Flocons d'avoine") {
		t.Errorf("render fr:\n%s", out)
	}
	// Re-extracting keeps translations.
	mustRun(t, "i18n", "extract", "-s", dir, "--lang", "fr")
	b, _ = os.ReadFile(tr)
	if !bytes.Contains(b, []byte("Flocons d'avoine")) {
		t.Error("extract lost a translation")
	}
	if out := mustRun(t, "i18n", "status", "-s", dir); !strings.Contains(out, "cranberry-baked-oatmeal") {
		t.Error(out)
	}
}

func TestImport(t *testing.T) {
	page := `<script type="application/ld+json">{"@type":"Recipe","name":"Toast","recipeYield":"2","recipeIngredient":["2 slices bread"],"recipeInstructions":"Toast it."}</script>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(page)) }))
	defer srv.Close()
	out := mustRun(t, "import", srv.URL)
	if !strings.Contains(out, "title: Toast") || !strings.Contains(out, "- 2 slices bread") {
		t.Error(out)
	}
}

func TestSkillsInstall(t *testing.T) {
	var tgz bytes.Buffer
	if err := skills.Pack("../../skills", &tgz); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz.Bytes()) }))
	defer srv.Close()
	t.Setenv("GET_COOKED_SKILLS_URL", srv.URL)
	dir := filepath.Join(t.TempDir(), "skills")
	// Without a terminal, installing requires --yes and prints the location first.
	out, code := run(t, "skills", "install", "--dir", dir)
	if code == 0 || !strings.Contains(out, dir) || !strings.Contains(out, "--yes") {
		t.Errorf("expected confirmation failure:\n%s", out)
	}
	out = mustRun(t, "skills", "install", "--dir", dir, "--yes")
	for _, s := range []string{"import-recipe", "recipe-illustration", "add-site-language", "translate-recipe"} {
		if _, err := os.Stat(filepath.Join(dir, s, "SKILL.md")); err != nil {
			t.Errorf("%s not installed:\n%s", s, out)
		}
	}
	mustRun(t, "skills", "install", "import-recipe", "--dir", dir, "--yes")
	if _, code := run(t, "skills", "install", "nope", "--dir", dir, "--yes"); code == 0 {
		t.Error("unknown skill should fail")
	}
	if out := mustRun(t, "skills", "list"); !strings.Contains(out, "translate-recipe") {
		t.Error(out)
	}
}

func TestSkillsSource(t *testing.T) {
	if s := skills.SourceFor("1.2.3"); !strings.Contains(s.URL, "/releases/download/v1.2.3/get-cooked-skills.tar.gz") {
		t.Error(s.URL)
	}
	for _, v := range []string{"dev", "0.1.1-0.20261118185420-abcdef123456"} {
		if s := skills.SourceFor(v); !strings.Contains(s.URL, "refs/heads/main") {
			t.Errorf("%s: %s", v, s.URL)
		}
	}
}

func TestScaffoldMatchesExample(t *testing.T) {
	got, _ := scaffold.File("ingredients.yaml")
	want, _ := os.ReadFile(filepath.Join(exampleSite, "ingredients.yaml"))
	strip := func(b []byte) string { return string(b[bytes.IndexByte(b, '\n'):]) }
	if strip(got) != strip(want) {
		t.Error("internal/scaffold/files/ingredients.yaml differs from examples/site/ingredients.yaml; copy it over")
	}
}
