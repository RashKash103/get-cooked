package recipe

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/RashKash103/get-cooked/internal/units"
	"github.com/RashKash103/get-cooked/schema"
)

func TestDensityEntry(t *testing.T) {
	tb := units.Default()
	db := NewDB()
	if ds := db.Add("milk", DBYAML{Density: &Density{Value: 1.03, Unit: "g/ml"}}, tb, Loc{}); len(ds) != 0 {
		t.Fatal(ds)
	}
	if g := db.Entries["milk"].Info.GramsPerML; math.Abs(g-1.03) > 1e-9 {
		t.Errorf("gpml = %v", g)
	}
	if ds := db.Add("y", DBYAML{Density: &Density{Value: 1, Unit: "g/bucket"}}, tb, Loc{}); len(ds) == 0 {
		t.Error("want unknown unit error")
	}
	if ds := db.Add("f", DBYAML{Density: &Density{Value: 125, Unit: "g/cup"}}, tb, Loc{}); len(ds) != 0 ||
		math.Abs(db.Entries["f"].Info.GramsPerML-125/tb.Units["cup"].Factor) > 1e-12 {
		t.Errorf("g/cup: %v", ds)
	}
}

func TestGramsPerCupMigrationMessage(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/ingredients.yaml"
	if err := os.WriteFile(p, []byte("flour:\n  grams_per_cup: 125\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, ds := LoadDB([]string{p}, units.Default())
	if !ds.HasErrors() || !strings.Contains(ds.Error(), "run `get-cooked migrate`") || ds[0].Line != 2 {
		t.Errorf("got %v", ds)
	}
}

// The schema's density unit enum must match units.DensityUnits exactly.
func TestDensityUnitsMatchSchema(t *testing.T) {
	b, err := schema.FS.ReadFile("ingredients.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Defs struct {
			Entry struct {
				Properties struct {
					Density struct {
						Properties struct {
							Unit struct {
								Enum []string `json:"enum"`
							} `json:"unit"`
						} `json:"properties"`
					} `json:"density"`
				} `json:"properties"`
			} `json:"entry"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	enum := doc.Defs.Entry.Properties.Density.Properties.Unit.Enum
	if len(enum) != len(units.DensityUnits) {
		t.Errorf("schema has %d density units, code has %d", len(enum), len(units.DensityUnits))
	}
	for _, u := range enum {
		if _, ok := units.DensityUnits[u]; !ok {
			t.Errorf("schema unit %q missing from units.DensityUnits", u)
		}
	}
}
