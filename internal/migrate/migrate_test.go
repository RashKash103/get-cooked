package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RashKash103/get-cooked/internal/config"
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
