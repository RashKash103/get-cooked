package recipe

import (
	"github.com/RashKash103/get-cooked/internal/expr"
	"github.com/RashKash103/get-cooked/internal/tmpl"
	"github.com/RashKash103/get-cooked/internal/units"
)

// Version of the compiled format shared with the TypeScript runtime.
const Version = 1

// Compiled is the runtime representation of a recipe. It is serialised into
// each recipe page and evaluated by both the Go and TypeScript runtimes.
type Compiled struct {
	V           int                   `json:"v"`
	System      string                `json:"system"`
	Servings    ServingsC             `json:"servings"`
	Options     []OptionC             `json:"options,omitempty"`
	Scale       *expr.Expr            `json:"scale,omitempty"`
	Vars        []VarC                `json:"vars,omitempty"`
	Times       []TimeC               `json:"times,omitempty"`
	Equipment   []EquipC              `json:"equipment,omitempty"`
	Ingredients []IngSectionC         `json:"ingredients"`
	Steps       []StepSectionC        `json:"steps"`
	Notes       []NoteC               `json:"notes,omitempty"`
	Images      []ImageC              `json:"images,omitempty"`
	Ings        map[string]*units.Ing `json:"ings,omitempty"`
	Units       *units.Table          `json:"units"`
}

// ServingsC configures the servings control.
type ServingsC struct {
	Default float64 `json:"default"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Step    float64 `json:"step"`
	One     string  `json:"one"`
	Other   string  `json:"other"`
}

// OptionC is a compiled option.
type OptionC struct {
	ID      string     `json:"id"`
	Type    string     `json:"type"`
	Label   string     `json:"label"`
	Help    string     `json:"help,omitempty"`
	Default any        `json:"default"`
	Choices []ChoiceC  `json:"choices,omitempty"`
	Min     *float64   `json:"min,omitempty"`
	Max     *float64   `json:"max,omitempty"`
	Step    *float64   `json:"step,omitempty"`
	Unit    string     `json:"unit,omitempty"`
	When    *expr.Expr `json:"when,omitempty"`
	Swap    string     `json:"swap,omitempty"` // a substitutes choice: the ingredient id it swaps
	L       Loc        `json:"-"`
}

// ChoiceC is a choice value.
type ChoiceC struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// VarC is a computed variable (in dependency order).
type VarC struct {
	ID string     `json:"id"`
	E  *expr.Expr `json:"e"`
	L  Loc        `json:"-"`
}

// TimeC is a named time in minutes.
type TimeC struct {
	ID    string     `json:"id"`
	Label string     `json:"label"`
	E     *expr.Expr `json:"e"`
	L     Loc        `json:"-"`
}

// EquipC is a piece of equipment.
type EquipC struct {
	ID   string        `json:"id"`
	When *expr.Expr    `json:"when,omitempty"`
	Name tmpl.Template `json:"name"`
	Note tmpl.Template `json:"note,omitempty"`
	L    Loc           `json:"-"`
}

// IngSectionC is a group of ingredients.
type IngSectionC struct {
	Title string     `json:"title,omitempty"`
	When  *expr.Expr `json:"when,omitempty"`
	Items []IngC     `json:"items"`
	L     Loc        `json:"-"`
}

// IngC is a compiled ingredient.
type IngC struct {
	ID       string        `json:"id"`
	Ref      string        `json:"ref,omitempty"`  // key into Compiled.Ings
	Base     string        `json:"base,omitempty"` // ingredient DB key when Ref is a per-recipe override
	Name     string        `json:"name"`
	Plural   string        `json:"plural,omitempty"`
	Unit     string        `json:"unit,omitempty"`
	Amount   *AmountC      `json:"amount,omitempty"`
	Fixed    bool          `json:"fixed,omitempty"`
	When     *expr.Expr    `json:"when,omitempty"`
	Note     tmpl.Template `json:"note,omitempty"`
	Optional bool          `json:"optional,omitempty"`
	Recipe   *RecipeRef    `json:"recipe,omitempty"`
	Split    []SplitC      `json:"split,omitempty"`
	Swap     string        `json:"swap,omitempty"` // the swap_<id> option when it has substitutes
	L        Loc           `json:"-"`
}

// SplitC is a named portion of an ingredient used in a particular step
// (`split: {dough: 4, topping: 2}` referenced as @salt.dough).
type SplitC struct {
	ID string  `json:"id"`
	A  AmountC `json:"a"`
}

// RecipeRef links an ingredient to another recipe (a prerequisite). The amount
// is measured in the target recipe's servings unit.
type RecipeRef struct {
	Slug  string  `json:"slug"`
	One   string  `json:"one"`
	Other string  `json:"other"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Step  float64 `json:"step"`
	With  []WithC `json:"with,omitempty"`
}

// WithC passes a value to one of the target recipe's options.
type WithC struct {
	ID   string     `json:"id"`
	Type string     `json:"type"`
	E    *expr.Expr `json:"e"`
}

// AmountC is either a number (scaled when S) or an expression.
//
// Hi is the upper end of a range ("1–2 cloves").
type AmountC struct {
	N  *float64   `json:"n,omitempty"`
	S  bool       `json:"s,omitempty"`
	E  *expr.Expr `json:"e,omitempty"`
	Hi *AmountC   `json:"hi,omitempty"`
}

// StepSectionC is a group of steps.
type StepSectionC struct {
	Title string     `json:"title,omitempty"`
	When  *expr.Expr `json:"when,omitempty"`
	Items []StepC    `json:"items"`
	L     Loc        `json:"-"`
}

// StepC is a compiled step.
type StepC struct {
	When  *expr.Expr    `json:"when,omitempty"`
	Text  tmpl.Template `json:"text"`
	Note  tmpl.Template `json:"note,omitempty"`
	Image string        `json:"image,omitempty"`
	L     Loc           `json:"-"`
}

// ImageC is an image variant. Src is the file name relative to the recipe
// until the site builder rewrites it to a URL (and fills SrcSet).
type ImageC struct {
	When   *expr.Expr `json:"when"`
	Src    string     `json:"src"`
	SrcSet string     `json:"srcset,omitempty"`
	Alt    string     `json:"alt,omitempty"`
	L      Loc        `json:"-"`
}

// NoteC is a compiled callout.
type NoteC struct {
	When *expr.Expr    `json:"when,omitempty"`
	Kind string        `json:"kind"`
	Text tmpl.Template `json:"text"`
	L    Loc           `json:"-"`
}

// Recipe is a compiled recipe plus its metadata.
type Recipe struct {
	Path        string
	Slug        string
	Title       string
	Description string
	Image       string
	ImageAlt    string
	Category    string
	Cuisine     string
	Tags        []string
	Author      string
	Source      *Source
	Date        string
	Draft       bool
	C           *Compiled
	// Categories used by the ingredient DB for each ingredient (by item id).
	IngCategories map[string]string
}
