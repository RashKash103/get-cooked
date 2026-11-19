package units

import (
	"testing"
)

var flour = &Ing{Name: "flour", GramsPerML: 125 / 236.5882365}
var milk = &Ing{Name: "milk", GramsPerML: 1.03, Measure: Volume}
var butter = &Ing{Name: "butter", GramsPerML: 0.959, UnitGrams: map[string]float64{"stick": 113}}
var egg = &Ing{Name: "egg", Plural: "eggs", GramsEach: 50, Round: &Rule{Denoms: []int{1}, Min: 1}}

func TestFormat(t *testing.T) {
	tb := Default()
	cases := []struct {
		amount float64
		unit   string
		system string
		ing    *Ing
		fixed  bool
		want   string
	}{
		{15.0 / 64, "tsp", Original, nil, false, "¼ tsp"},
		{0.125, "cup", Original, nil, false, "2 tbsp"},
		{1.0 / 3, "tbsp", Original, nil, false, "1 tsp"},
		{5, "tbsp", Original, nil, false, "⅓ cup"},
		{3, "tbsp", Original, nil, false, "3 tbsp"},
		{4, "tbsp", Original, nil, false, "¼ cup"},
		{6, "tbsp", Original, nil, false, "6 tbsp"},
		{2.3, "cup", Original, nil, false, "2 ⅓ cups"},
		{1, "cup", Original, nil, false, "1 cup"},
		{0.5, "cup", Original, nil, false, "½ cup"},
		{1.5, "cup", Original, nil, false, "1 ½ cups"},
		{0.02, "tsp", Original, nil, false, "pinch"},
		{1.25, "tbsp", Original, nil, false, "3 ¾ tsp"},
		{2, "quart", Original, nil, false, "2 quarts"},
		{0.1, "quart", Original, nil, false, "6 ½ tbsp"},
		{20, "oz", Original, nil, false, "1 ¼ lb"},
		{12, "oz", Original, nil, false, "12 oz"},
		{1250, "g", Original, nil, false, "1.25 kg"},
		{123, "g", Original, nil, false, "123 g"},
		{7.3, "g", Original, nil, false, "7.5 g"},
		{0.1, "g", Original, nil, false, "pinch"},
		{2.25, "cup", Metric, flour, false, "281 g"},
		{1, "cup", Metric, milk, false, "237 ml"},
		{1, "cup", Metric, nil, false, "237 ml"},
		{100, "g", US, nil, false, "3 ½ oz"},
		{500, "g", US, milk, false, "2 cups"},
		{2, "cup", US, flour, false, "2 cups"},
		{1, "stick", Metric, butter, false, "113 g"},
		{1, "stick", Original, butter, false, "1 stick"},
		{3, "stick", US, butter, false, "3 sticks"},
		{0.4, "each", Original, egg, false, "1"},
		{2.6, "each", Original, egg, false, "3"},
		{1.4, "can", Original, nil, false, "1 ½ cans"},
		{0.1, "can", Original, nil, false, "¼ can"},
		{0.3, "cup", Original, nil, true, "⅓ cup"},
		{3, "tsp", Original, nil, true, "3 tsp"},
		{1, "tsp", Metric, nil, false, "5 ml"},
		{0.875, "cup", Original, nil, false, "¾ cup + 2 tbsp"},
		{10, "tbsp", Original, nil, false, "⅔ cup"},
		{9, "tbsp", Original, nil, false, "½ cup + 1 tbsp"},
		{1.125, "cup", Original, nil, false, "1 cup + 2 tbsp"},
		{7, "tbsp", Original, nil, false, "7 tbsp"},
	}
	for _, c := range cases {
		got := tb.Format(c.amount, c.unit, c.system, c.ing, c.fixed).String()
		if got != c.want {
			t.Errorf("Format(%v %s, %s) = %q, want %q", c.amount, c.unit, c.system, got, c.want)
		}
	}
}

func TestFormatRange(t *testing.T) {
	tb := Default()
	cases := []struct {
		lo, hi float64
		unit   string
		want   string
	}{
		{2, 3, "clove", "2–3 cloves"},
		{0.5, 1, "tsp", "½–1 tsp"},
		{1, 3, "tsp", "1 tsp – 1 tbsp"},
		{1, 1.01, "cup", "1 cup"},
		{1, 2, "each", "1–2"},
	}
	for _, c := range cases {
		if got := tb.FormatRange(c.lo, c.hi, c.unit, Original, nil, false).String(); got != c.want {
			t.Errorf("FormatRange(%v, %v, %s) = %q want %q", c.lo, c.hi, c.unit, got, c.want)
		}
	}
}

func TestLookup(t *testing.T) {
	tb := Default()
	for in, want := range map[string]string{
		"T": "tbsp", "t": "tsp", "Tablespoons": "tbsp", "cups": "cup", "g": "g", "grams": "g",
		"lbs": "lb", "Cloves": "clove", "": "each", "fl oz": "floz", "tsp.": "tsp", "leaves": "leaf",
	} {
		got, ok := tb.Lookup(in)
		if !ok || got.ID != want {
			t.Errorf("Lookup(%q) = %v, want %s", in, got, want)
		}
	}
	if _, ok := tb.Lookup("cpu"); ok {
		t.Error("cpu should be unknown")
	}
	tb.AddCountUnit("tortilla", "")
	if x, ok := tb.Lookup("tortillas"); !ok || x.ID != "tortilla" {
		t.Error("custom unit")
	}
	if err := tb.Validate(); err != nil {
		t.Error(err)
	}
	tb.Ladders["us.volume"] = append(tb.Ladders["us.volume"], Rung{Unit: "g", Rule: Rule{Denoms: []int{2}}})
	if err := tb.Validate(); err == nil {
		t.Error("expected ladder dimension error")
	}
}

func TestFraction(t *testing.T) {
	tb := Default()
	for in, want := range map[float64]string{
		0.5: "½", 2.25: "2 ¼", 1: "1", 0.375: "⅜", 1.0 / 3: "⅓", 0.3: "0.3", 2.0 / 3: "⅔", -0.5: "-½",
	} {
		if got := tb.FormatFraction(in); got != want {
			t.Errorf("FormatFraction(%v) = %q want %q", in, got, want)
		}
	}
	tb.Fractions = "ascii"
	if got := tb.FormatFraction(2.75); got != "2 3/4" {
		t.Errorf("ascii = %q", got)
	}
	tb.Fractions = "native"
	tb.Digits = "٠١٢٣٤٥٦٧٨٩"
	if got := tb.FormatFraction(2.75); got != "٢\u200e ٣\u200e⁄٤\u200e" {
		t.Errorf("native = %q", got)
	}
	tb.Glyphs = map[string]string{"3/4": "¾"}
	if got := tb.FormatFraction(2.75); got != "٢\u200e ¾" {
		t.Errorf("glyph override = %q", got)
	}
	tb.Glyphs, tb.Digits = nil, ""
	tb.Fractions = "decimal"
	if got := tb.FormatFraction(2.75); got != "2.75" {
		t.Errorf("decimal = %q", got)
	}
}

func TestTempLengthDuration(t *testing.T) {
	cases := []struct{ got, want string }{
		{Temp(350, false, Original), "350°F"},
		{Temp(350, false, Metric), "175°C"},
		{Temp(425, false, Metric), "220°C"},
		{Temp(180, true, Original), "180°C"},
		{Temp(180, true, US), "350°F"},
		{Temp(100, true, US), "210°F"},
		{Length(9, false, Original), "9 in"},
		{Length(9, false, Metric), "23 cm"},
		{Length(20, true, US), "8 in"},
		{Duration(45), "45 min"},
		{Duration(60), "1 hr"},
		{Duration(75.4), "1 hr 15 min"},
		{Duration(0.5), "30 sec"},
		{Duration(2.5), "2 min 30 sec"},
		{Duration(1.99), "2 min"},
		{Duration(12.5), "13 min"},
		{Duration(0), "0 min"},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("%d: got %q want %q", i, c.got, c.want)
		}
	}
}
