package recipe

import (
	"errors"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/RashKash103/get-cooked/internal/ingline"
	"github.com/RashKash103/get-cooked/internal/units"
)

// DBYAML is an ingredient database entry as written in ingredients.yaml.
type DBYAML struct {
	Name        string             `yaml:"name"`
	Plural      string             `yaml:"plural"`
	Aliases     []string           `yaml:"aliases"`
	GramsPerCup float64            `yaml:"grams_per_cup"`
	GramsEach   float64            `yaml:"grams_each"`
	UnitGrams   map[string]float64 `yaml:"unit_grams"`
	Measure     string             `yaml:"measure"`
	Round       *RoundYAML         `yaml:"round"`
	Category    string             `yaml:"category"`
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
	info := &units.Ing{Name: e.Name, Plural: e.Plural, GramsEach: y.GramsEach, Measure: y.Measure}
	if y.GramsPerCup > 0 {
		info.GramsPerML = y.GramsPerCup / tb.Units["cup"].Factor
	}
	for name, g := range y.UnitGrams {
		x, ok := tb.Lookup(name)
		if !ok {
			diags = append(diags, loc.Diag("ingredient %q: unknown unit %q in unit_grams", key, name))
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
	e.Info = info
	db.Entries[key] = e
	for _, n := range append([]string{key, e.Name, e.Plural}, e.Aliases...) {
		if n != "" {
			db.byName[norm(n)] = key
		}
	}
	return diags
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
