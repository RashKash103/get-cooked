package ingline

import (
	"testing"

	"github.com/RashKash103/get-cooked/internal/units"
)

func TestParse(t *testing.T) {
	tb := units.Default()
	cases := []struct {
		in   string
		want Line
	}{
		{"2 1/4 cups all-purpose flour, sifted", Line{Amount: 2.25, Unit: "cup", Name: "all-purpose flour", Note: "sifted"}},
		{"1½ tsp kosher salt", Line{Amount: 1.5, Unit: "tsp", Name: "kosher salt"}},
		{"½ teaspoon baking soda", Line{Amount: 0.5, Unit: "tsp", Name: "baking soda"}},
		{"3 large eggs", Line{Amount: 3, Name: "large eggs"}},
		{"2-3 cloves garlic, minced", Line{Amount: 2, AmountMax: 3, Unit: "clove", Name: "garlic", Note: "minced"}},
		{"1 (14 oz) can diced tomatoes", Line{Amount: 1, Unit: "can", Name: "diced tomatoes", Note: "14 oz"}},
		{"Salt and pepper to taste", Line{Name: "Salt and pepper", Note: "to taste"}},
		{"1 cup (240 ml) whole milk", Line{Amount: 1, Unit: "cup", Name: "whole milk", Note: "240 ml"}},
		{"4 fl oz cream", Line{Amount: 4, Unit: "floz", Name: "cream"}},
		{"pinch of nutmeg", Line{Amount: 1, Unit: "pinch", Name: "nutmeg"}},
		{"200g butter", Line{Amount: 200, Unit: "g", Name: "butter"}},
		{"1.5 lbs chicken thighs", Line{Amount: 1.5, Unit: "lb", Name: "chicken thighs"}},
		{"3/4 c. sugar", Line{Amount: 0.75, Unit: "cup", Name: "sugar"}},
		{"- 2 T olive oil", Line{Amount: 2, Unit: "tbsp", Name: "olive oil"}},
		{"1 cup walnuts (optional)", Line{Amount: 1, Unit: "cup", Name: "walnuts", Note: "optional"}},
	}
	for _, c := range cases {
		got := Parse(c.in, tb)
		if got != c.want {
			t.Errorf("Parse(%q)\n got %+v\nwant %+v", c.in, got, c.want)
		}
	}
}

func TestSingular(t *testing.T) {
	for in, want := range map[string]string{
		"eggs": "egg", "large eggs": "large egg", "cherries": "cherry", "tomatoes": "tomato",
		"peaches": "peach", "molasses": "molasses", "asparagus": "asparagus", "gas": "gas",
	} {
		if got := Singular(in); got != want {
			t.Errorf("Singular(%q) = %q want %q", in, got, want)
		}
	}
}
