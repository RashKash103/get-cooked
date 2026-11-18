package runtime

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/RashKash103/get-cooked/internal/config"
	"github.com/RashKash103/get-cooked/internal/i18n"
	"github.com/RashKash103/get-cooked/internal/recipe"
)

var update = flag.Bool("update", false, "update golden files")

const exampleSite = "../../examples/site"

var (
	exampleOnce sync.Once
	examples    map[string]*recipe.Recipe
	exampleErr  error
)

// compileExample compiles every example recipe (so recipe references link)
// and returns the one at rel (relative to the recipes directory).
func compileExample(t *testing.T, rel string) *recipe.Recipe {
	t.Helper()
	exampleOnce.Do(func() {
		cfg, err := config.Load(exampleSite)
		if err != nil {
			exampleErr = err
			return
		}
		tb, err := cfg.UnitTable()
		if err != nil {
			exampleErr = err
			return
		}
		db, ds := recipe.LoadDB([]string{cfg.Path("ingredients.yaml")}, tb)
		if ds.HasErrors() {
			exampleErr = ds
			return
		}
		c := &recipe.Compiler{Units: tb, DB: db, System: cfg.Units.DefaultSystem}
		files, _ := filepath.Glob(filepath.Join(exampleSite, "recipes", "*", "*.yaml"))
		examples = map[string]*recipe.Recipe{}
		var all []*recipe.Recipe
		for _, f := range files {
			if i18n.TranslationLang(f) != "" {
				continue
			}
			r, ds := c.CompileFile(f)
			if ds.HasErrors() {
				exampleErr = ds
				return
			}
			rel, _ := filepath.Rel(filepath.Join(exampleSite, "recipes"), f)
			examples[filepath.ToSlash(rel)] = r
			all = append(all, r)
		}
		if ds := recipe.Link(all); ds.HasErrors() {
			exampleErr = ds
		}
	})
	if exampleErr != nil {
		t.Fatal(exampleErr)
	}
	r, ok := examples[rel]
	if !ok {
		t.Fatalf("no example %s", rel)
	}
	return r
}

func render(t *testing.T, r *recipe.Recipe, st State) *Model {
	t.Helper()
	m, err := Render(r.C, st)
	if err != nil {
		t.Fatalf("render %+v: %v", st, err)
	}
	return m
}

func stepText(m *Model) string {
	var lines []string
	for _, s := range m.Steps {
		for _, st := range s.Items {
			var sb strings.Builder
			for _, seg := range st.Segs {
				if seg.T == "ing" && seg.Q != "" {
					sb.WriteString(seg.Q + seg.J)
				}
				sb.WriteString(seg.V)
			}
			lines = append(lines, sb.String())
		}
	}
	return strings.Join(lines, "\n")
}

func ingredient(m *Model, id string) *IngM {
	for _, s := range m.Ingredients {
		for i := range s.Items {
			if s.Items[i].ID == id {
				return &s.Items[i]
			}
		}
	}
	return nil
}

// The scenarios from the original feature request.
func TestOatmealScenarios(t *testing.T) {
	r := compileExample(t, "breakfast/cranberry-baked-oatmeal.yaml")

	m := render(t, r, State{Servings: 3})
	if m.Equipment[0].Name != "13×9 inch baking dish" {
		t.Errorf("3 servings pan = %q", m.Equipment[0].Name)
	}
	if got := m.Times[1]; got.Minutes != 36 {
		t.Errorf("bake time at 3 servings = %v, want 30 + 3*2", got.Minutes)
	}
	m = render(t, r, State{Servings: 2})
	if m.Equipment[0].Name != "8×8 inch baking dish" {
		t.Errorf("2 servings pan = %q", m.Equipment[0].Name)
	}
	for _, n := range m.Notes {
		if strings.Contains(n.Text, "softer") {
			t.Errorf("unexpected softness warning at 2 servings: %s", n.Text)
		}
	}
	m = render(t, r, State{Servings: 1})
	soft := false
	for _, n := range m.Notes {
		soft = soft || n.Kind == "warning" && strings.Contains(n.Text, "softer")
	}
	if !soft {
		t.Errorf("1 serving should warn about a softer result, got %+v", m.Notes)
	}

	// Cranberries default on: a dedicated step exists and the mixing step mentions them.
	on := stepText(render(t, r, State{}))
	off := stepText(render(t, r, State{Options: map[string]any{"cranberries": false}}))
	if !strings.Contains(on, "Scatter the remaining 2 tbsp dried cranberries") || !strings.Contains(on, "fold in 6 tbsp dried cranberries") {
		t.Errorf("cranberry steps missing:\n%s", on)
	}
	if strings.Contains(off, "cranberr") {
		t.Errorf("cranberries should be gone:\n%s", off)
	}
	if strings.Count(on, "\n") != strings.Count(off, "\n")+1 {
		t.Error("toggling cranberries should add exactly one step")
	}
	// Cranberries are divided between two steps, so the list shows both shares.
	if u := ingredient(render(t, r, State{}), "cranberries").Uses; len(u) != 2 || u[0].N != 3 || u[1].Q != "2 tbsp" {
		t.Errorf("cranberry uses = %+v", u)
	}
	if ingredient(render(t, r, State{Options: map[string]any{"cranberries": false}}), "cranberries") != nil {
		t.Error("cranberries should be removed from the ingredient list")
	}
}

func TestVariantsAndUnits(t *testing.T) {
	r := compileExample(t, "breakfast/buttermilk-pancakes.yaml")
	m := render(t, r, State{})
	if d := ingredient(m, "dairy"); d == nil || d.Name != "buttermilk" || d.Q.String() != "2 cups" {
		t.Errorf("dairy = %+v", d)
	}
	m = render(t, r, State{Options: map[string]any{"no_buttermilk": true}, System: "metric"})
	if d := ingredient(m, "dairy"); d == nil || d.Name != "milk" || d.Q.String() != "414 ml" {
		t.Errorf("dairy (no buttermilk, metric) = %+v %v", d, d.Q)
	}
	if f := ingredient(m, "all_purpose_flour"); f == nil || f.Q.String() != "250 g" {
		t.Errorf("flour metric = %+v", f.Q)
	}
	if e := ingredient(render(t, r, State{Servings: 1}), "eggs"); e.Q.String() != "1" || e.Name != "egg" {
		t.Errorf("eggs at 1 serving = %v %s", e.Q, e.Name)
	}
}

func TestOptionStatesAndServings(t *testing.T) {
	r := compileExample(t, "mains/crispy-chicken-thighs.yaml")
	states := OptionStates(r.C.Options)
	if len(states) != 2*4 {
		t.Errorf("states = %d", len(states))
	}
	sv := SweepServings(recipe.ServingsC{Min: 1, Max: 100, Step: 1, Default: 4})
	if sv[0] != 1 || sv[len(sv)-1] != 100 || len(sv) > 30 {
		t.Errorf("servings sweep = %v", sv)
	}
	m := render(t, r, State{Options: map[string]any{"heat": 0.0}})
	if ingredient(m, "cayenne") != nil {
		t.Error("cayenne should disappear at heat 0")
	}
}

func TestRenderErrors(t *testing.T) {
	compile := func(src string) *recipe.Recipe {
		t.Helper()
		c := &recipe.Compiler{Units: compileExample(t, "breakfast/buttermilk-pancakes.yaml").C.Units, DB: recipe.NewDB()}
		r, ds := c.Compile("t.yaml", []byte(src))
		if ds.HasErrors() {
			t.Fatal(ds)
		}
		return r
	}
	cases := []struct{ src, want string }{
		{`title: x
servings: 2
options: [{id: a, type: toggle}]
ingredients: [{id: s, name: salt, when: a}]
steps: [Add @s.]`, "@s is referenced but not included"},
		{`title: x
servings: 2
ingredients: [{id: s, name: salt, amount: "servings - 3"}]
steps: [go]`, "is negative"},
		{`title: x
servings: 2
ingredients: [{id: s, name: salt, when: "servings > 1"}, {id: s, name: pepper, when: "servings > 2"}]
steps: [go]`, "included twice"},
		{`title: x
servings: 2
ingredients: [{id: s, name: salt, when: "servings"}]
steps: [go]`, "condition must be true/false"},
		{`title: x
servings: 2
vars: {d: "1 / (servings - 2)"}
ingredients: [salt]
steps: [go]`, "division by zero"},
	}
	for _, c := range cases {
		r := compile(c.src)
		ds := Sweep(r)
		if len(ds) == 0 || !strings.Contains(ds.Error(), c.want) {
			t.Errorf("want %q, got %v", c.want, ds)
		}
	}
}

// TestConformanceGoldens writes golden render results for every example
// recipe. The TypeScript runtime replays them (web/src/runtime/conformance.test.ts).
func TestConformanceGoldens(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(exampleSite, "recipes", "*", "*.yaml"))
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no example recipes")
	}
	for _, f := range files {
		if i18n.TranslationLang(f) != "" {
			continue
		}
		rel, _ := filepath.Rel(filepath.Join(exampleSite, "recipes"), f)
		r := compileExample(t, rel)
		type tc struct {
			State State  `json:"state"`
			Model *Model `json:"model"`
		}
		var cases []tc
		opts := OptionStates(r.C.Options)
		servings := SweepServings(r.C.Servings)
		systems := []string{"original", "us", "metric"}
		// Deterministic sample: walk the cartesian product with a stride.
		total := len(opts) * len(servings) * len(systems)
		stride := max(1, total/16)
		for i := 0; i < total; i += stride {
			st := State{
				Options:  opts[i%len(opts)],
				Servings: servings[(i/len(opts))%len(servings)],
				System:   systems[(i/(len(opts)*len(servings)))%len(systems)],
			}
			cases = append(cases, tc{st, render(t, r, st)})
		}
		cases = append(cases, tc{State{}, render(t, r, State{})})
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		if err := enc.Encode(map[string]any{"compiled": r.C, "cases": cases}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("..", "..", "testdata", "conformance", r.Slug+".json")
		if *update {
			if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(want, buf.Bytes()) {
			t.Errorf("golden %s out of date; run: go test ./internal/runtime -update", path)
		}
	}
}

func TestCompiledRoundTrip(t *testing.T) {
	r := compileExample(t, "desserts/chocolate-chip-cookies.yaml")
	b, err := json.Marshal(r.C)
	if err != nil {
		t.Fatal(err)
	}
	var back recipe.Compiled
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	for _, st := range []State{{}, {Servings: 60, System: "metric", Options: map[string]any{"rest": "none", "brown_butter": false}}} {
		a, _ := json.Marshal(render(t, r, st))
		bm, err := Render(&back, st)
		if err != nil {
			t.Fatal(err)
		}
		b2, _ := json.Marshal(bm)
		if string(a) != string(b2) {
			t.Errorf("round trip differs for %+v", st)
		}
	}
}
