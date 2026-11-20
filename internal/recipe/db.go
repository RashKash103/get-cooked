package recipe

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/RashKash103/get-cooked/internal/ingline"
	"github.com/RashKash103/get-cooked/internal/units"
)

// DBYAML is an ingredient database entry as written in ingredients.yaml.
type DBYAML struct {
	Name      string             `yaml:"name"`
	Plural    string             `yaml:"plural"`
	Aliases   []string           `yaml:"aliases"`
	Density   *Density           `yaml:"density"`
	GramsEach float64            `yaml:"grams_each"`
	UnitGrams map[string]float64 `yaml:"unit_grams"`
	Measure   string             `yaml:"measure"`
	Round     *RoundYAML         `yaml:"round"`
	Category  string             `yaml:"category"`
	Ladder    []LadderStep       `yaml:"ladder"`
}

// LadderStep is one unit of an ingredient's own display ladder: a unit name,
// or {unit, denoms, tiers, min, max, tiny} to change its rounding.
type LadderStep units.Rung

// UnmarshalYAML accepts "tbsp" or a mapping.
func (s *LadderStep) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		s.Unit = n.Value
		return nil
	}
	var r units.Rung
	if err := n.Decode(&r); err != nil {
		return err
	}
	*s = LadderStep(r)
	return nil
}

// Density is a density with an explicit unit, e.g. {value: 1.03, unit: g/ml}.
type Density struct {
	Value float64 `yaml:"value"`
	Unit  string  `yaml:"unit"`
}

// RoundYAML is the rounding rule for count quantities.
type RoundYAML struct {
	Denoms []int   `yaml:"denoms"`
	Step   float64 `yaml:"step"`
	Min    float64 `yaml:"min"`
}

// DBEntry is a resolved ingredient database entry.
type DBEntry struct {
	Key      string
	Name     string
	Plural   string
	Aliases  []string
	Category string
	Info     *units.Ing
}

// DB is the ingredient database.
type DB struct {
	Entries map[string]*DBEntry
	byName  map[string]string
}

// NewDB returns an empty database.
func NewDB() *DB { return &DB{Entries: map[string]*DBEntry{}, byName: map[string]string{}} }

// LoadDB reads ingredient database files. Missing files are ignored.
func LoadDB(paths []string, tb *units.Table) (*DB, Diags) {
	db := NewDB()
	var diags Diags
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			diags = append(diags, Diag{File: p, Msg: err.Error()})
			continue
		}
		var root yaml.Node
		if err := yaml.Unmarshal(b, &root); err != nil {
			diags = append(diags, Diag{File: p, Msg: err.Error()})
			continue
		}
		if len(root.Content) > 0 {
			for _, kv := range orderedPairs(root.Content[0]) {
				for _, f := range orderedPairs(kv[1]) {
					if f[0].Value == "grams_per_cup" {
						diags = append(diags, Diag{File: p, Line: f[0].Line, Col: f[0].Column,
							Msg: fmt.Sprintf("%s: grams_per_cup was replaced by density in 0.3.0; run `get-cooked migrate` (or write `density: { value: %s, unit: g/cup }`)", kv[0].Value, f[1].Value)})
					}
				}
			}
		}
		if diags.HasErrors() {
			continue
		}
		diags = append(diags, ValidateSchema("ingredients", p, &root)...)
		if diags.HasErrors() {
			continue
		}
		if len(root.Content) == 0 {
			continue
		}
		for _, kv := range orderedPairs(root.Content[0]) {
			var y DBYAML
			if err := kv[1].Decode(&y); err != nil {
				diags = append(diags, Diag{File: p, Line: kv[1].Line, Msg: err.Error()})
				continue
			}
			ds := db.Add(kv[0].Value, y, tb, Loc{p, kv[0].Line, kv[0].Column})
			diags = append(diags, ds...)
		}
	}
	return db, diags
}

// Add inserts an entry (later entries override earlier ones).
func (db *DB) Add(key string, y DBYAML, tb *units.Table, loc Loc) Diags {
	var diags Diags
	e := &DBEntry{Key: key, Name: y.Name, Plural: y.Plural, Aliases: y.Aliases, Category: y.Category}
	if e.Name == "" {
		e.Name = strings.ReplaceAll(key, "-", " ")
	}
	info, ds := ApplyIngredientFields(&units.Ing{Name: e.Name, Plural: e.Plural}, y, tb, loc, key)
	diags = append(diags, ds...)
	e.Info = info
	db.Entries[key] = e
	for _, n := range append([]string{key, e.Name, e.Plural}, e.Aliases...) {
		if n != "" {
			db.byName[norm(n)] = key
		}
	}
	return diags
}

// ApplyIngredientFields returns a copy of base with the conversion fields of
// y applied (density, grams_each, unit_grams, measure, round, category). It
// is used for ingredient database entries and for per-recipe overrides.
func ApplyIngredientFields(base *units.Ing, y DBYAML, tb *units.Table, loc Loc, what string) (*units.Ing, Diags) {
	var diags Diags
	cp := *base
	info := &cp
	if base.UnitGrams != nil {
		info.UnitGrams = make(map[string]float64, len(base.UnitGrams))
		for k, v := range base.UnitGrams {
			info.UnitGrams[k] = v
		}
	}
	if y.GramsEach > 0 {
		info.GramsEach = y.GramsEach
	}
	if y.Measure != "" {
		info.Measure = y.Measure
	}
	if y.Category != "" {
		info.Cat = y.Category
	}
	if d := y.Density; d != nil {
		f, ok := units.DensityUnits[d.Unit]
		switch {
		case !ok:
			diags = append(diags, loc.Diag("ingredient %q: unknown density unit %q", what, d.Unit))
		case d.Value <= 0:
			diags = append(diags, loc.Diag("ingredient %q: density must be positive", what))
		default:
			info.GramsPerML = d.Value * f
		}
	}
	for name, g := range y.UnitGrams {
		x, ok := tb.Lookup(name)
		if !ok {
			diags = append(diags, loc.Diag("ingredient %q: unknown unit %q in unit_grams", what, name))
			continue
		}
		if info.UnitGrams == nil {
			info.UnitGrams = map[string]float64{}
		}
		info.UnitGrams[x.ID] = g
	}
	if r := y.Round; r != nil {
		rule := units.Rule{Denoms: r.Denoms, Min: r.Min}
		if r.Step > 0 {
			rule.Tiers = []units.Tier{{Step: r.Step}}
		}
		if len(rule.Denoms) == 0 && len(rule.Tiers) == 0 {
			rule.Denoms = []int{1}
		}
		info.Round = &rule
	}
	if y.Ladder != nil { // an empty list turns an inherited ladder off
		steps := make([]units.Rung, len(y.Ladder))
		for i, s := range y.Ladder {
			steps[i] = units.Rung(s)
		}
		rungs, err := tb.IngredientLadder(steps, info)
		if err != nil {
			diags = append(diags, loc.Diag("ingredient %q: %v", what, err))
		} else {
			info.Ladder = rungs
		}
	}
	return info, diags
}

func norm(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, "-", " "))), " ")
}

// Match finds an entry by key, name, plural or alias (case-insensitive, also
// trying the singular form).
func (db *DB) Match(name string) *DBEntry {
	if e, ok := db.Entries[name]; ok {
		return e
	}
	n := norm(name)
	if k, ok := db.byName[n]; ok {
		return db.Entries[k]
	}
	if k, ok := db.byName[norm(ingline.Singular(n))]; ok {
		return db.Entries[k]
	}
	return nil
}

// Keys returns entry keys sorted.
func (db *DB) Keys() []string {
	out := make([]string, 0, len(db.Entries))
	for k := range db.Entries {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Clone returns a shallow copy that can take recipe-local additions.
func (db *DB) Clone() *DB {
	c := NewDB()
	for k, v := range db.Entries {
		c.Entries[k] = v
	}
	for k, v := range db.byName {
		c.byName[k] = v
	}
	return c
}
