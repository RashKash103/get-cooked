// Package units converts, scales, rounds and formats ingredient quantities.
//
// All behaviour is driven by a data Table that is serialised into each page so
// the TypeScript runtime applies exactly the same rules.
package units

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/RashKash103/get-cooked/internal/expr"
)

// Dimensions and systems.
const (
	Volume = "volume"
	Mass   = "mass"
	Count  = "count"

	US       = "us"
	Metric   = "metric"
	Original = "original"
)

// Unit is a unit of measure. Factor converts to the dimension's base unit
// (ml for volume, g for mass, 1 for count units).
type Unit struct {
	ID      string   `json:"id"`
	Dim     string   `json:"dim"`
	System  string   `json:"sys,omitempty"`
	Factor  float64  `json:"f"`
	One     string   `json:"one"`
	Other   string   `json:"other"`
	Aliases []string `json:"-"`
}

// Tier rounds values below Below to a multiple of Step (Below 0 = no limit).
type Tier struct {
	Below float64 `json:"below,omitempty" yaml:"below"`
	Step  float64 `json:"step" yaml:"step"`
}

// Rule describes how to round a value expressed in some unit. Either Denoms
// (round to the nearest fraction with one of these denominators) or Tiers
// (decimal steps by magnitude) is used.
type Rule struct {
	Denoms []int   `json:"denoms,omitempty" yaml:"denoms"`
	Tiers  []Tier  `json:"tiers,omitempty" yaml:"tiers"`
	Min    float64 `json:"min,omitempty" yaml:"min"`   // smallest value this unit may display (ladders) / clamp (ingredients)
	Max    float64 `json:"max,omitempty" yaml:"max"`   // above this, split into the next larger unit + remainder (ladders)
	Tiny   string  `json:"tiny,omitempty" yaml:"tiny"` // label when a positive amount rounds to zero
}

// Rung is one step of a display ladder.
type Rung struct {
	Unit string `json:"unit" yaml:"unit"`
	Rule `yaml:",inline"`
}

// Table holds all unit data and display rules.
type Table struct {
	Units     map[string]*Unit  `json:"units"`
	Ladders   map[string][]Rung `json:"ladders"` // key: "<system>.<dim>", rungs ascending
	Tolerance float64           `json:"tol"`
	Fractions string            `json:"fractions"`        // unicode | ascii | decimal
	Count     Rule              `json:"count"`            // default rounding for count units
	Fallback  Rule              `json:"fallback"`         // rounding for units outside any ladder
	Decimal   string            `json:"dec,omitempty"`    // decimal separator ("." when empty)
	Words     map[string]string `json:"words,omitempty"`  // duration words: hr, min, sec
	Of        string            `json:"of,omitempty"`     // word between a measured amount and a name ("de")
	Digits    string            `json:"digits,omitempty"` // ten digits replacing 0–9 ("٠١٢٣٤٥٦٧٨٩")
	Glyphs    map[string]string `json:"glyphs,omitempty"` // custom fraction characters, keyed "1/2"
	aliases   map[string]string
}

// DensityUnits maps each supported density unit to grams per millilitre for
// one of that unit. The ingredient schema's density.unit enum must list
// exactly these keys (checked by a test).
var DensityUnits = func() map[string]float64 {
	const (
		tsp    = 4.92892159375
		tbsp   = 14.78676478125
		cup    = 236.5882365
		floz   = 29.5735295625
		pint   = 473.176473
		quart  = 946.352946
		gallon = 3785.411784
		oz     = 28.349523125
		lb     = 453.59237
	)
	return map[string]float64{
		"g/ml": 1, "g/cm3": 1, "g/l": 0.001, "kg/l": 1, "kg/m3": 0.001,
		"g/tsp": 1 / tsp, "g/tbsp": 1 / tbsp, "g/cup": 1 / cup,
		"oz/tbsp": oz / tbsp, "oz/cup": oz / cup, "oz/floz": oz / floz, "oz/pint": oz / pint,
		"lb/cup": lb / cup, "lb/pint": lb / pint, "lb/quart": lb / quart, "lb/gal": lb / gallon,
	}
}()

// Ing carries the per-ingredient data needed for conversion and rounding.
type Ing struct {
	Name       string             `json:"name"`
	Plural     string             `json:"plural,omitempty"`
	GramsPerML float64            `json:"gpml,omitempty"`
	GramsEach  float64            `json:"each,omitempty"`
	UnitGrams  map[string]float64 `json:"ug,omitempty"`
	Measure    string             `json:"measure,omitempty"` // volume | mass preference
	Round      *Rule              `json:"round,omitempty"`
	Cat        string             `json:"cat,omitempty"` // shopping category
}

// Qty is a formatted quantity.
type Qty struct {
	Value float64 `json:"v"`
	Hi    float64 `json:"h,omitempty"` // upper end of a range, in the same unit
	Text  string  `json:"t"`
	Unit  string  `json:"u"`
	ID    string  `json:"id"` // display unit id ("" for bare counts)
}

// Max is the larger end of the quantity.
func (q Qty) Max() float64 { return math.Max(q.Value, q.Hi) }

// Join returns the text placed between this quantity and an ingredient name:
// the table's connector word when the quantity has a unit ("2 tazas de"),
// otherwise a space ("2 huevos").
func (t *Table) Join(q *Qty) string {
	if q == nil || t.Of == "" || q.Unit == "" || q.ID == "each" {
		return " "
	}
	return " " + t.Of + " "
}

// String joins number and unit.
func (q Qty) String() string {
	switch {
	case q.Text == "":
		return q.Unit
	case q.Unit == "":
		return q.Text
	}
	return q.Text + " " + q.Unit
}

func u(id, dim, sys string, f float64, one, other string, aliases ...string) *Unit {
	return &Unit{ID: id, Dim: dim, System: sys, Factor: f, One: one, Other: other, Aliases: aliases}
}

// Default returns the built-in table.
func Default() *Table {
	list := []*Unit{
		u("tsp", Volume, US, 4.92892159375, "tsp", "tsp", "teaspoon", "teaspoons", "t", "tsps"),
		u("tbsp", Volume, US, 14.78676478125, "tbsp", "tbsp", "tablespoon", "tablespoons", "T", "tbs", "tbsps", "tbl"),
		u("cup", Volume, US, 236.5882365, "cup", "cups", "cups", "c"),
		u("floz", Volume, US, 29.5735295625, "fl oz", "fl oz", "fl oz", "fl. oz", "fluid ounce", "fluid ounces"),
		u("pint", Volume, US, 473.176473, "pint", "pints", "pints", "pt"),
		u("quart", Volume, US, 946.352946, "quart", "quarts", "quarts", "qt"),
		u("gallon", Volume, US, 3785.411784, "gallon", "gallons", "gallons", "gal"),
		u("ml", Volume, Metric, 1, "ml", "ml", "milliliter", "milliliters", "millilitre", "millilitres", "mL", "cm3", "cm³", "cc"),
		u("l", Volume, Metric, 1000, "l", "l", "liter", "liters", "litre", "litres", "L", "dm3", "dm³"),
		u("g", Mass, Metric, 1, "g", "g", "gram", "grams", "gr"),
		u("kg", Mass, Metric, 1000, "kg", "kg", "kilogram", "kilograms", "kilo", "kilos"),
		u("oz", Mass, US, 28.349523125, "oz", "oz", "ounce", "ounces"),
		u("lb", Mass, US, 453.59237, "lb", "lb", "pound", "pounds", "lbs"),
		u("each", Count, "", 1, "", "", "ea", "whole", ""),
	}
	for _, c := range [][2]string{
		{"pinch", "pinches"}, {"piece", "pieces"}, {"dash", "dashes"}, {"clove", "cloves"}, {"can", "cans"},
		{"slice", "slices"}, {"package", "packages"}, {"bunch", "bunches"}, {"sprig", "sprigs"},
		{"handful", "handfuls"}, {"stick", "sticks"}, {"head", "heads"}, {"leaf", "leaves"},
		{"sheet", "sheets"}, {"drop", "drops"}, {"jar", "jars"}, {"bottle", "bottles"},
		{"stalk", "stalks"}, {"fillet", "fillets"}, {"packet", "packets"}, {"bag", "bags"},
		{"envelope", "envelopes"}, {"box", "boxes"}, {"knob", "knobs"}, {"scoop", "scoops"},
	} {
		list = append(list, u(c[0], Count, "", 1, c[0], c[1], c[1]))
	}
	metricTiers := []Tier{{Below: 10, Step: 0.5}, {Below: 1000, Step: 1}, {Step: 5}}
	t := &Table{
		Units: map[string]*Unit{},
		Ladders: map[string][]Rung{
			US + "." + Volume: {
				{"tsp", Rule{Denoms: []int{2, 4, 8}, Tiny: "pinch"}},
				{"tbsp", Rule{Denoms: []int{2}, Min: 1, Max: 8}},
				{"cup", Rule{Denoms: []int{2, 3, 4}, Min: 0.25}},
			},
			US + "." + Mass: {
				{"oz", Rule{Denoms: []int{2, 4}, Tiny: "pinch"}},
				{"lb", Rule{Denoms: []int{2, 4}, Min: 1}},
			},
			Metric + "." + Volume: {
				{"ml", Rule{Tiers: metricTiers, Tiny: "dash"}},
				{"l", Rule{Tiers: []Tier{{Step: 0.05}}, Min: 1}},
			},
			Metric + "." + Mass: {
				{"g", Rule{Tiers: metricTiers, Tiny: "pinch"}},
				{"kg", Rule{Tiers: []Tier{{Step: 0.05}}, Min: 1}},
			},
		},
		Tolerance: 0.1,
		Fractions: "unicode",
		Count:     Rule{Denoms: []int{2, 4}},
		Fallback:  Rule{Denoms: []int{2, 3, 4, 8}},
	}
	for _, x := range list {
		t.Units[x.ID] = x
	}
	t.index()
	return t
}

func (t *Table) index() {
	t.aliases = map[string]string{}
	ids := make([]string, 0, len(t.Units))
	for id := range t.Units {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		x := t.Units[id]
		for _, a := range append([]string{x.ID, x.One, x.Other}, x.Aliases...) {
			t.aliases[a] = x.ID
			if _, taken := t.aliases[strings.ToLower(a)]; !taken {
				t.aliases[strings.ToLower(a)] = x.ID
			}
		}
	}
}

// AddCountUnit registers a custom count unit (e.g. "tortilla").
func (t *Table) AddCountUnit(one, other string) {
	if other == "" {
		other = one + "s"
	}
	t.Units[one] = u(one, Count, "", 1, one, other, other)
	t.index()
}

// Lookup resolves a unit name or alias. Case-sensitive matches win so that
// "T" (tablespoon) and "t" (teaspoon) stay distinct.
func (t *Table) Lookup(name string) (*Unit, bool) {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	if id, ok := t.aliases[name]; ok {
		return t.Units[id], true
	}
	if id, ok := t.aliases[strings.ToLower(name)]; ok {
		return t.Units[id], true
	}
	return nil, false
}

// Names returns all accepted unit spellings (for suggestions).
func (t *Table) Names() []string {
	out := make([]string, 0, len(t.aliases))
	for k := range t.aliases {
		if k != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Validate checks that ladders refer to known units in the right dimension.
func (t *Table) Validate() error {
	for key, rungs := range t.Ladders {
		for _, r := range rungs {
			x, ok := t.Units[r.Unit]
			if !ok {
				return fmt.Errorf("ladder %s: unknown unit %q", key, r.Unit)
			}
			if !strings.HasSuffix(key, "."+x.Dim) {
				return fmt.Errorf("ladder %s: unit %q is %s", key, r.Unit, x.Dim)
			}
			if len(r.Denoms) == 0 && len(r.Tiers) == 0 {
				return fmt.Errorf("ladder %s: unit %q needs denoms or tiers", key, r.Unit)
			}
		}
	}
	return nil
}

// Round rounds x according to the rule. The result is 0 when x is positive
// but too small to display with this rule.
func (r Rule) Round(x float64) float64 {
	if len(r.Tiers) > 0 {
		step := r.Tiers[len(r.Tiers)-1].Step
		for _, tier := range r.Tiers {
			if tier.Below == 0 || math.Abs(x) < tier.Below {
				step = tier.Step
				break
			}
		}
		return expr.RoundTo(math.Round(x/step)*step, 6)
	}
	denoms := r.Denoms
	if len(denoms) == 0 {
		denoms = []int{1}
	}
	best, bestErr, bestD := 0.0, math.Inf(1), 0
	for _, d := range denoms {
		c := math.Round(x*float64(d)) / float64(d)
		e := math.Abs(c - x)
		if e < bestErr-1e-12 || math.Abs(e-bestErr) <= 1e-12 && d < bestD {
			best, bestErr, bestD = c, e, d
		}
	}
	return expr.RoundTo(best, 6)
}

func relErr(r, x float64) float64 {
	if x == 0 {
		return 0
	}
	return math.Abs(r-x) / math.Abs(x)
}

var unicodeFractions = map[[2]int]string{
	{1, 2}: "½", {1, 3}: "⅓", {2, 3}: "⅔", {1, 4}: "¼", {3, 4}: "¾",
	{1, 8}: "⅛", {3, 8}: "⅜", {5, 8}: "⅝", {7, 8}: "⅞",
	{1, 5}: "⅕", {2, 5}: "⅖", {3, 5}: "⅗", {4, 5}: "⅘", {1, 6}: "⅙", {5, 6}: "⅚",
}

// FormatFraction prints x as a mixed fraction when it is (very close to) a
// fraction with denominator ≤ 8, otherwise as a decimal.
var digitRunRe = regexp.MustCompile(`[0-9]+(?:[.,٫٬][0-9]+)*`)

// L replaces ASCII digits with the table's digits (if any).
func (t *Table) L(s string) string {
	if t.Digits == "" {
		return s
	}
	d := []rune(t.Digits)
	if len(d) != 10 {
		return s
	}
	// Non-Latin digits are "Arabic numbers" to the Unicode bidi algorithm: a
	// space or dash between two of them counts as right-to-left, which would
	// show "١ ١⁄٢" as "١⁄٢ ١" in a left-to-right page. A left-to-right mark
	// after each run of digits keeps the logical order in LTR and RTL text.
	// (Same regexp as localDigits in web/src/runtime/units.ts.)
	s = digitRunRe.ReplaceAllString(s, "${0}\u200e")
	var sb strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			sb.WriteRune(d[r-'0'])
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// Num formats a number with the table's decimal separator and digits.
func (t *Table) Num(x float64) string {
	s := expr.FormatNumber(x)
	if t.Decimal != "" && t.Decimal != "." {
		s = strings.Replace(s, ".", t.Decimal, 1)
	}
	return t.L(s)
}

func (t *Table) FormatFraction(x float64) string {
	if t.Fractions == "decimal" {
		return t.Num(x)
	}
	neg := x < 0
	ax := math.Abs(x)
	whole := math.Floor(ax + 1e-9)
	frac := ax - whole
	if frac < 1e-6 {
		return t.Num(x)
	}
	for d := 2; d <= 8; d++ {
		n := math.Round(frac * float64(d))
		if n == 0 || n == float64(d) || math.Abs(frac*float64(d)-n) > 1e-4 {
			continue
		}
		g := gcd(int(n), d)
		num, den := int(n)/g, d/g
		var f string
		if g, ok := t.Glyphs[fmt.Sprintf("%d/%d", num, den)]; ok {
			f = g // custom glyph for this language
		} else {
			switch t.Fractions {
			case "ascii":
				f = t.L(fmt.Sprintf("%d/%d", num, den))
			case "native":
				// The language's own numerals around a fraction slash: ١⁄٢
				f = t.L(fmt.Sprintf("%d\u2044%d", num, den))
			default:
				u, ok := unicodeFractions[[2]int{num, den}]
				if !ok {
					return t.Num(x)
				}
				f = u
			}
		}
		s := f
		if whole > 0 {
			s = t.Num(whole) + " " + f
		}
		if neg {
			s = "-" + s
		}
		return s
	}
	return t.Num(x)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func (t *Table) text(r Rule, v float64) string {
	if len(r.Tiers) > 0 {
		return t.Num(v)
	}
	return t.FormatFraction(v)
}

func label(x *Unit, v float64) string {
	if v > 1 {
		return x.Other
	}
	return x.One
}

// Format converts amount (expressed in unitID) for the given unit system and
// returns a rounded, human friendly quantity. ing may be nil.
func (t *Table) Format(amount float64, unitID, system string, ing *Ing, fixed bool) Qty {
	src, ok := t.Units[unitID]
	if !ok {
		return Qty{Value: amount, Text: t.FormatFraction(t.Fallback.Round(amount)), Unit: unitID, ID: unitID}
	}
	if src.Dim == Count {
		return t.formatCount(amount, src, system, ing)
	}
	dim, sys := src.Dim, src.System
	base := amount * src.Factor // ml or g
	if !fixed {
		// measure (mass|volume) is a display preference in every system; with
		// no preference, metric shows weight whenever the density is known.
		want := ""
		if ing != nil && ing.GramsPerML > 0 {
			want = ing.Measure
			if want == "" && system == Metric {
				want = Mass
			}
		}
		switch system {
		case Metric, US:
			sys = system
		}
		switch {
		case want == Mass && dim == Volume:
			dim, base = Mass, base*ing.GramsPerML
		case want == Volume && dim == Mass:
			dim, base = Volume, base/ing.GramsPerML
		}
	}
	if fixed || sys == src.System && dim == src.Dim && !t.inLadder(sys, dim, src.ID) {
		// Keep the authored unit if it rounds cleanly (e.g. quarts, fl oz).
		x := amount
		r := t.ruleFor(sys, dim, src.ID)
		v := r.Round(x)
		if fixed || v > 0 && relErr(v, x) <= t.Tolerance {
			return Qty{Value: v, Text: t.text(r, v), Unit: label(src, v), ID: src.ID}
		}
	}
	return t.ladder(base, sys, dim)
}

// FormatRange formats a range such as "1–2 cloves" or "½–1 tsp". The unit is
// chosen for the high end; when the two ends end up in different units the
// range is written in full ("2 tsp – 1 tbsp").
func (t *Table) FormatRange(lo, hi float64, unitID, system string, ing *Ing, fixed bool) Qty {
	qh := t.Format(hi, unitID, system, ing, fixed)
	ql := t.Format(lo, unitID, system, ing, fixed)
	if ql.String() == qh.String() {
		return qh
	}
	if ql.ID == qh.ID && ql.Text != "" && qh.Text != "" && !strings.Contains(qh.Text, "+") && !strings.Contains(ql.Text, "+") {
		return Qty{Value: ql.Value, Hi: qh.Value, Text: ql.Text + "–" + qh.Text, Unit: qh.Unit, ID: qh.ID}
	}
	return Qty{Value: ql.Value, Hi: qh.Value, Text: ql.String() + " – " + qh.String(), ID: qh.ID}
}

func (t *Table) inLadder(sys, dim, id string) bool {
	for _, r := range t.Ladders[sys+"."+dim] {
		if r.Unit == id {
			return true
		}
	}
	return false
}

func (t *Table) ruleFor(sys, dim, id string) Rule {
	for _, r := range t.Ladders[sys+"."+dim] {
		if r.Unit == id {
			return r.Rule
		}
	}
	return t.Fallback
}

// ladder picks the largest unit that represents base cleanly. A unit whose
// value would exceed its Max is split into the next larger unit plus a
// remainder ("¾ cup + 2 tbsp").
func (t *Table) ladder(base float64, sys, dim string) Qty {
	rungs := t.Ladders[sys+"."+dim]
	if len(rungs) == 0 {
		// No ladder: use the base unit of the dimension.
		id := "g"
		if dim == Volume {
			id = "ml"
		}
		x := t.Units[id]
		v := t.Fallback.Round(base / x.Factor)
		return Qty{Value: v, Text: t.FormatFraction(v), Unit: label(x, v), ID: id}
	}
	for i := len(rungs) - 1; i >= 0; i-- {
		r := rungs[i]
		x := t.Units[r.Unit]
		val := base / x.Factor
		v := r.Round(val)
		if v <= 0 || v < r.Min || relErr(v, val) > t.Tolerance {
			continue
		}
		if r.Max > 0 && v > r.Max && i+1 < len(rungs) {
			if q, ok := t.compound(base, r, rungs[i+1]); ok {
				return q
			}
		}
		return Qty{Value: v, Text: t.text(r.Rule, v), Unit: label(x, v), ID: x.ID}
	}
	// Nothing fits within tolerance: use the smallest rung.
	r := rungs[0]
	x := t.Units[r.Unit]
	val := base / x.Factor
	v := r.Round(val)
	if v == 0 && val > 0 {
		if r.Tiny != "" {
			return Qty{Value: 0, Text: "", Unit: r.Tiny, ID: x.ID}
		}
		v = smallest(r.Rule)
	}
	return Qty{Value: v, Text: t.text(r.Rule, v), Unit: label(x, v), ID: x.ID}
}

func (t *Table) compound(base float64, small, big Rung) (Qty, bool) {
	x, bx := t.Units[small.Unit], t.Units[big.Unit]
	whole := floorTo(big.Rule, base/bx.Factor)
	if whole <= 0 || whole < big.Min {
		return Qty{}, false
	}
	rest := small.Round((base - whole*bx.Factor) / x.Factor)
	if rest <= 0 {
		return Qty{Value: whole, Text: t.text(big.Rule, whole), Unit: label(bx, whole), ID: bx.ID}, true
	}
	text := t.text(big.Rule, whole) + " " + label(bx, whole) + " + " + t.text(small.Rule, rest)
	return Qty{Value: expr.RoundTo(whole+rest*x.Factor/bx.Factor, 6), Text: text, Unit: label(x, rest), ID: bx.ID}, true
}

// floorTo rounds x down to the largest value representable by the rule.
func floorTo(r Rule, x float64) float64 {
	best := 0.0
	for _, d := range r.Denoms {
		if c := math.Floor(x*float64(d)+1e-9) / float64(d); c > best {
			best = c
		}
	}
	return expr.RoundTo(best, 6)
}

func smallest(r Rule) float64 {
	if len(r.Tiers) > 0 {
		return r.Tiers[0].Step
	}
	m := 1
	for _, d := range r.Denoms {
		if d > m {
			m = d
		}
	}
	return 1 / float64(m)
}

func (t *Table) formatCount(amount float64, src *Unit, system string, ing *Ing) Qty {
	if system == Metric && ing != nil && ing.UnitGrams[src.ID] > 0 {
		return t.ladder(amount*ing.UnitGrams[src.ID], Metric, Mass)
	}
	r := t.Count
	if ing != nil && ing.Round != nil {
		r = *ing.Round
	}
	v := r.Round(amount)
	if v < r.Min {
		v = r.Min
	}
	if v == 0 && amount > 0 {
		v = smallest(r)
	}
	return Qty{Value: v, Text: t.text(r, v), Unit: label(src, v), ID: src.ID}
}

// Temp formats a temperature given in Fahrenheit (fromC=false) or Celsius.
func Temp(v float64, fromC bool, system string) string {
	toC := system == Metric || system == Original && fromC
	switch {
	case toC && fromC, !toC && !fromC:
		return expr.FormatNumber(math.Round(v)) + "°" + map[bool]string{true: "C", false: "F"}[toC]
	case toC:
		c := (v - 32) * 5 / 9
		return expr.FormatNumber(math.Round(c/5)*5) + "°C"
	default:
		f := v*9/5 + 32
		step := 5.0
		if f > 250 {
			step = 25
		}
		return expr.FormatNumber(math.Round(f/step)*step) + "°F"
	}
}

// Length formats a length given in inches (fromCM=false) or centimetres.
func Length(v float64, fromCM bool, system string) string {
	toCM := system == Metric || system == Original && fromCM
	switch {
	case toCM && fromCM:
		return expr.FormatNumber(v) + " cm"
	case !toCM && !fromCM:
		return plainFrac.FormatFraction(math.Round(v*2)/2) + " in"
	case toCM:
		return expr.FormatNumber(math.Round(v*2.54)) + " cm"
	default:
		return plainFrac.FormatFraction(math.Round(v/2.54*2)/2) + " in"
	}
}

var plainFrac = &Table{Fractions: "unicode"}

// Duration formats minutes using the table's duration words.
func (t *Table) Duration(min float64) string {
	return t.L(durationWords(min, t.Words))
}

// Duration formats minutes as "1 hr 5 min", "45 min" or "30 sec".
func Duration(min float64) string { return durationWords(min, nil) }

func durationWords(min float64, words map[string]string) string {
	w := func(k string) string {
		if v := words[k]; v != "" {
			return v
		}
		return k
	}
	if min <= 0 {
		return "0 " + w("min")
	}
	if min < 1 {
		return expr.FormatNumber(math.Max(5, math.Round(min*60/5)*5)) + " " + w("sec")
	}
	secs := int(math.Round(min * 60))
	if min < 10 && secs%60 != 0 {
		sec := int(math.Round(float64(secs%60)/5) * 5)
		m := secs / 60
		if sec == 60 {
			m, sec = m+1, 0
		}
		if sec == 0 {
			return fmt.Sprintf("%d %s", m, w("min"))
		}
		return fmt.Sprintf("%d %s %d %s", m, w("min"), sec, w("sec"))
	}
	total := int(math.Round(min))
	h, m := total/60, total%60
	switch {
	case h == 0:
		return fmt.Sprintf("%d %s", m, w("min"))
	case m == 0:
		return fmt.Sprintf("%d %s", h, w("hr"))
	default:
		return fmt.Sprintf("%d %s %d %s", h, w("hr"), m, w("min"))
	}
}
