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

func TestInlineIngredientOverrides(t *testing.T) {
	base := compileExample(t, "breakfast/buttermilk-pancakes.yaml")
	c := &recipe.Compiler{Units: base.C.Units, DB: exampleDB(t), System: "original"}
	r, ds := c.Compile("t.yaml", []byte(`title: T
servings: 4
ingredients:
  - { id: flour, ingredient: all-purpose-flour, amount: 2, unit: cup, measure: mass }
  - { id: flour2, ingredient: all-purpose-flour, amount: 2, unit: cup }
  - { id: berries, name: wild blueberries, amount: 1, unit: cup, density: { value: 140, unit: g/cup }, category: produce }
  - { id: eggs, ingredient: egg, amount: 2, round: { denoms: [2] } }
steps: [Mix @flour and @berries.]
`))
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	m := render(t, r, State{})
	if q := ingredient(m, "flour").Q.String(); q != "8 ¾ oz" {
		t.Errorf("flour with measure: mass = %q", q)
	}
	if q := ingredient(m, "flour2").Q.String(); q != "2 cups" {
		t.Errorf("flour without override = %q (the database entry must not change)", q)
	}
	if q := ingredient(render(t, r, State{System: "metric"}), "berries").Q.String(); q != "140 g" {
		t.Errorf("berries with inline density, metric = %q", q)
	}
	if q := ingredient(render(t, r, State{Servings: 1}), "eggs").Q.String(); q != "½" {
		t.Errorf("eggs with inline rounding at 1 serving = %q", q)
	}
	var fl recipe.IngC
	for _, it := range r.C.Ingredients[0].Items {
		if it.ID == "flour" {
			fl = it
		}
	}
	if fl.Base != "all-purpose-flour" || fl.Ref == "all-purpose-flour" || r.C.Ings[fl.Ref].Measure != "mass" || r.C.Ings["all-purpose-flour"].Measure != "" {
		t.Errorf("override bookkeeping: %+v", fl)
	}
	if r.IngCategories["berries"] != "produce" {
		t.Error("category override")
	}
}

func exampleDB(t *testing.T) *recipe.DB {
	t.Helper()
	cfg, err := config.Load(exampleSite)
	if err != nil {
		t.Fatal(err)
	}
	tb, err := cfg.UnitTable()
	if err != nil {
		t.Fatal(err)
	}
	db, ds := recipe.LoadDB([]string{cfg.Path("ingredients.yaml")}, tb)
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	return db
}

func TestSubstitutes(t *testing.T) {
	base := compileExample(t, "breakfast/buttermilk-pancakes.yaml")
	c := &recipe.Compiler{Units: base.C.Units, DB: exampleDB(t), System: "original"}
	r, ds := c.Compile("t.yaml", []byte(`title: T
servings: 4
ingredients:
  - id: dairy
    ingredient: buttermilk
    amount: [1.5, 2]
    unit: cup
    subs:
      - { ingredient: milk, note: plus 2 tbsp lemon juice }
      - { ingredient: heavy-cream, ratio: 0.5 }
      - 3 tbsp maple syrup
  - 1 egg
steps: ["Add @dairy{if swap_dairy == 'milk'} with the lemon{end}."]
`))
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	var opt *recipe.OptionC
	for i := range r.C.Options {
		if r.C.Options[i].ID == "swap_dairy" {
			opt = &r.C.Options[i]
		}
	}
	if opt == nil || opt.Type != "choice" || opt.Default != "buttermilk" || opt.Swap != "dairy" || opt.Label != "Swap buttermilk" || len(opt.Choices) != 4 {
		t.Fatalf("swap option: %+v", opt)
	}
	if ids := []string{opt.Choices[1].ID, opt.Choices[2].ID, opt.Choices[3].ID}; ids[0] != "milk" || ids[1] != "heavy_cream" || ids[2] != "maple_syrup" {
		t.Errorf("choice ids %v", ids)
	}
	for _, c := range []struct {
		pick, ing, step string
	}{
		{"buttermilk", "1 ½–2 cups buttermilk", "Add 1 ½–2 cups buttermilk."},
		{"milk", "1 ½–2 cups milk", "Add 1 ½–2 cups milk with the lemon."},
		{"heavy_cream", "¾–1 cup heavy cream", "Add ¾–1 cup heavy cream."},
		{"maple_syrup", "3 tbsp maple syrup", "Add 3 tbsp maple syrup."},
	} {
		m := render(t, r, State{Options: map[string]any{"swap_dairy": c.pick}})
		it := ingredient(m, "dairy")
		if got := it.Q.String() + it.J + it.Name; got != c.ing {
			t.Errorf("%s: ingredient %q want %q", c.pick, got, c.ing)
		}
		if it.Swap != "swap_dairy" {
			t.Errorf("%s: swap %q", c.pick, it.Swap)
		}
		if got := stepText(m); got != c.step {
			t.Errorf("%s: step %q want %q", c.pick, got, c.step)
		}
	}
	if it := ingredient(render(t, r, State{}), "egg"); it.Swap != "" {
		t.Error("an ingredient without substitutes has no swap")
	}

	for src, want := range map[string]string{
		"subs: [{ id: other, ingredient: milk }]":           "shares the id",
		"subs: [{ ingredient: milk, when: 'true' }]":        "can't have its own `when:`",
		"subs: [{ ingredient: milk, amount: 1, ratio: 2 }]": "use it without `amount:`",
		"subs: [{ ingredient: milk, optional: true }]":      "can't be optional",
	} {
		_, ds := c.Compile("t.yaml", []byte("title: T\nservings: 4\ningredients:\n  - { id: dairy, ingredient: buttermilk, amount: 2, unit: cup, "+src+" }\nsteps: [Add @dairy.]\n"))
		if !strings.Contains(ds.Error(), want) {
			t.Errorf("%s: want error containing %q, got %v", src, want, ds)
		}
	}
}

func TestCombinationSubstitutes(t *testing.T) {
	base := compileExample(t, "breakfast/buttermilk-pancakes.yaml")
	c := &recipe.Compiler{Units: base.C.Units, DB: exampleDB(t), System: "original"}
	r, ds := c.Compile("t.yaml", []byte(`title: T
servings: 4
ingredients:
  - id: dairy
    ingredient: buttermilk
    amount: 2
    unit: cup
    subs:
      - [{ ingredient: milk, ratio: 0.875 }, { ingredient: lemon-juice, amount: 2, unit: tbsp }]
      - label: Yogurt, thinned
        items: [{ ingredient: heavy-cream, ratio: 0.5 }, { ingredient: water, ratio: 0.5 }, { id: tang, ingredient: lemon-juice, amount: 1, unit: tsp }]
steps: [Whisk in @dairy., "Use @dairy{1/2} now, the rest later; then the @dairy{-}."]
`))
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	var opt recipe.OptionC
	for _, o := range r.C.Options {
		if o.ID == "swap_dairy" {
			opt = o
		}
	}
	if len(opt.Choices) != 3 || opt.Choices[1].ID != "milk_lemon_juice" || opt.Choices[1].Label != "milk + lemon juice" || opt.Choices[2].Label != "Yogurt, thinned" {
		t.Fatalf("choices %+v", opt.Choices)
	}
	ids := func(m *Model) string {
		var out []string
		for _, s := range m.Ingredients {
			for _, it := range s.Items {
				out = append(out, it.ID+"="+it.Q.String()+it.J+it.Name)
			}
		}
		return strings.Join(out, "; ")
	}
	for _, c := range []struct{ pick, ings, steps string }{
		{"buttermilk", "dairy=2 cups buttermilk", "Whisk in 2 cups buttermilk.\nUse 1 cup buttermilk now, the rest later; then the buttermilk."},
		{"milk_lemon_juice", "dairy=1 ¾ cups milk; dairy_lemon_juice=2 tbsp lemon juice",
			"Whisk in 1 ¾ cups milk and 2 tbsp lemon juice.\nUse ¾ cup + 2 tbsp milk and 1 tbsp lemon juice now, the rest later; then the milk and lemon juice."},
		{opt.Choices[2].ID, "dairy=1 cup heavy cream; dairy_water=1 cup water; tang=1 tsp lemon juice",
			"Whisk in 1 cup heavy cream, 1 cup water and 1 tsp lemon juice.\nUse ½ cup heavy cream, ½ cup water and ½ tsp lemon juice now, the rest later; then the heavy cream, water and lemon juice."},
	} {
		m := render(t, r, State{Options: map[string]any{"swap_dairy": c.pick}})
		if got := ids(m); got != c.ings {
			t.Errorf("%s ingredients:\n got %s\nwant %s", c.pick, got, c.ings)
		}
		if got := stepText(m); got != c.steps {
			t.Errorf("%s steps:\n got %s\nwant %s", c.pick, got, c.steps)
		}
	}
}

func TestMakes(t *testing.T) {
	base := compileExample(t, "breakfast/buttermilk-pancakes.yaml")
	c := &recipe.Compiler{Units: base.C.Units, DB: exampleDB(t), System: "original"}
	compile := func(path, src string) *recipe.Recipe {
		t.Helper()
		r, ds := c.Compile(path, []byte(src))
		if ds.HasErrors() {
			t.Fatal(ds)
		}
		return r
	}
	spice := compile("garam-masala.yaml", `title: Garam Masala
servings: { default: 1, min: 1, max: 4, unit: batch, plural: batches }
makes: { amount: "1/2", unit: cup, name: garam masala, density: { value: 100, unit: g/cup } }
ingredients: [1 tbsp cumin seeds]
steps: [Grind.]
`)
	// Explicit: measured by what garam-masala makes.
	curry := compile("curry.yaml", `title: Curry
servings: { default: 4, max: 100 }
ingredients:
  - { id: gm, recipe: garam-masala, amount: 2, unit: tsp }
steps: [Add @gm.]
`)
	if ds := recipe.Link([]*recipe.Recipe{spice, curry}); ds.HasErrors() {
		t.Fatal(ds)
	}
	m := render(t, curry, State{})
	it := m.Ingredients[0].Items[0]
	if it.Q.String() != "2 tsp" || it.Name != "garam masala" || it.Link == nil || it.Link.Servings != 1 {
		t.Fatalf("measured ref: %+v %+v link %+v", it, it.Q, it.Link)
	}
	// 64 servings: 32 tsp ≈ ⅔ cup, more than one ½-cup batch.
	if m := render(t, curry, State{Servings: 64}); m.Ingredients[0].Items[0].Link.Servings != 2 {
		t.Errorf("64 servings: want 2 batches, got %+v", m.Ingredients[0].Items[0].Link)
	}
	// The density from makes converts it to grams.
	if q := render(t, curry, State{System: "metric"}).Ingredients[0].Items[0].Q; q.ID != "g" {
		t.Errorf("metric: %+v", q)
	}
	if mk := render(t, spice, State{Servings: 2}).Makes; mk == nil || mk.String() != "1 cup" {
		t.Errorf("makes at 2 batches: %+v", mk)
	}

	// A measured ref to a recipe without makes is an error; so is an
	// unconvertible unit.
	plain := compile("plain.yaml", "title: Plain\nservings: 2\ningredients: [1 egg]\nsteps: [Go.]\n")
	bad := compile("bad.yaml", `title: Bad
servings: 1
ingredients:
  - { id: a, recipe: plain, amount: 1, unit: tsp }
  - { id: b, recipe: garam-masala, amount: 1, unit: clove }
steps: [Add @a and @b.]
`)
	ds := recipe.Link([]*recipe.Recipe{spice, plain, bad})
	if len(ds) != 2 || !strings.Contains(ds[0].Msg, "doesn't say what it makes") || !strings.Contains(ds[1].Msg, "can't be converted") {
		t.Errorf("diags: %v", ds)
	}
}
