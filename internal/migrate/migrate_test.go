package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RashKash103/get-cooked/internal/config"
	"github.com/RashKash103/get-cooked/internal/i18n"
	"github.com/RashKash103/get-cooked/internal/site"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDensityMigration(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "ingredients.yaml"), `# My ingredients
flour:
  name: flour
  grams_per_cup: 125   # King Arthur says 120
sugar: { grams_per_cup: 200, category: baking }
`)
	write(t, filepath.Join(dir, "recipes", "cake.yaml"), `title: Cake
servings: 2
ingredient_db:
  cocoa:
    grams_per_cup: 85.5
ingredients:
  - { id: cocoa, ingredient: cocoa, amount: 1, unit: cup }
steps: [Mix @cocoa.]
`)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The old format fails validation and points at the migration.
	s, err := site.Load(dir, site.LoadOptions{})
	if err == nil && !s.Diags.HasErrors() {
		t.Fatal("old format should not validate")
	}

	changes, err := Run(cfg, true)
	if err != nil || len(changes) != 2 {
		t.Fatalf("dry run: %v %+v", err, changes)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "ingredients.yaml")); !strings.Contains(string(b), "grams_per_cup") {
		t.Fatal("dry run must not write")
	}

	changes, err = Run(cfg, false)
	if err != nil || len(changes) != 2 || changes[0].Count+changes[1].Count != 3 {
		t.Fatalf("run: %v %+v", err, changes)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "ingredients.yaml"))
	got := string(b)
	for _, want := range []string{
		"# My ingredients",
		"  density: { value: 125, unit: g/cup }   # King Arthur says 120",
		"sugar: { density: { value: 200, unit: g/cup }, category: baking }",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	r, _ := os.ReadFile(filepath.Join(dir, "recipes", "cake.yaml"))
	if !strings.Contains(string(r), "density: { value: 85.5, unit: g/cup }") {
		t.Errorf("recipe-local db not migrated:\n%s", r)
	}

	// The migrated site validates, and running again changes nothing.
	s, err = site.Load(dir, site.LoadOptions{Sweep: true})
	if err != nil || s.Diags.HasErrors() {
		t.Fatalf("after migration: %v %v", err, s.Diags)
	}
	if changes, _ := Run(cfg, false); len(changes) != 0 {
		t.Errorf("second run changed %+v", changes)
	}
}

func TestMigrationsAreWellFormed(t *testing.T) {
	ids := map[string]bool{}
	for _, m := range All {
		if m.ID == "" || m.Since == "" || m.Description == "" || len(m.Kinds) == 0 || m.Apply == nil {
			t.Errorf("incomplete migration %+v", m)
		}
		if ids[m.ID] {
			t.Errorf("duplicate migration id %s", m.ID)
		}
		ids[m.ID] = true
	}
}

func TestI18nNumeralsMigration(t *testing.T) {
	apply := func(src string) (string, int) {
		t.Helper()
		for _, m := range All {
			if m.ID == "0.3.0-i18n-numerals" {
				out, n := m.Apply([]byte(src))
				return string(out), n
			}
		}
		t.Fatal("migration not registered")
		return "", 0
	}

	// A catalog written before these settings existed: all four are added in
	// the template's position (after `of`, before `system`), with comments.
	old := "# Spanish\n\nname: Español\ndir: ltr\ndecimal: \",\"\nof: de\nsystem: metric\n\nui:\n  skip: Ir al contenido\n"
	got, n := apply(old)
	if n != 4 {
		t.Fatalf("added %d keys:\n%s", n, got)
	}
	want := "of: de\n# Digits for numbers"
	if !strings.Contains(got, want) {
		t.Errorf("digits should follow `of`:\n%s", got)
	}
	order := []string{"of: de", "digits: \"\"", "fractions: \"\"", "fraction_glyphs: {}", "fraction_display: inline", "system: metric", "ui:"}
	pos := -1
	for _, s := range order {
		i := strings.Index(got, s)
		if i <= pos {
			t.Errorf("%q out of order in:\n%s", s, got)
		}
		pos = i
	}
	if !strings.HasPrefix(got, "# Spanish\n\nname: Español\n") || !strings.Contains(got, "\nui:\n  skip: Ir al contenido\n") {
		t.Errorf("existing text changed:\n%s", got)
	}
	// Running again changes nothing.
	if again, n := apply(got); n != 0 || again != got {
		t.Errorf("not idempotent (%d)", n)
	}

	// Keys the file already defines are left exactly as they are.
	custom := "name: हिन्दी\nof: \"\"\ndigits: deva\nfraction_display: diagonal\nui: {}\n"
	got, n = apply(custom)
	if n != 2 || strings.Count(got, "digits:") != 1 || !strings.Contains(got, "digits: deva") || !strings.Contains(got, "fraction_display: diagonal") {
		t.Errorf("custom (%d):\n%s", n, got)
	}
	if !strings.Contains(got, "digits: deva\n# Fraction style") {
		t.Errorf("fractions should follow the existing digits line:\n%s", got)
	}

	// No preceding single-line key: insert before the next one, above its comments.
	minimal := "# UI strings\nui:\n  skip: x\n"
	got, n = apply(minimal)
	if n != 4 || !strings.HasSuffix(got, "fraction_display: inline\n# UI strings\nui:\n  skip: x\n") {
		t.Errorf("minimal (%d):\n%s", n, got)
	}

	// The migrated catalog still loads, with behaviour-neutral defaults.
	dir := t.TempDir()
	migrated, _ := apply(old)
	write(t, filepath.Join(dir, "i18n", "es.yaml"), migrated)
	cat, err := i18n.Load(dir, "es")
	if err != nil {
		t.Fatal(err)
	}
	if cat.Digits != "" || cat.Fractions != "" || cat.FracDisplay != "inline" || cat.Decimal != "," || cat.System != "metric" {
		t.Errorf("catalog after migration: %+v", cat)
	}

	// A file that already has everything is untouched.
	if _, n := apply(string(i18n.EnglishYAML)); n != 0 {
		t.Errorf("template itself changed (%d)", n)
	}
}
