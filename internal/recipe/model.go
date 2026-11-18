package recipe

import (
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Src is a YAML scalar that remembers its position and whether it was a
// number, so "amount: 2" (scaled) and "amount: '2 * scale'" (raw expression)
// can be told apart.
type Src struct {
	Text  string
	Num   *float64
	Bool  *bool
	Line  int
	Col   int
	Style yaml.Style
	Set   bool
	// Comment is a trailing "# …" comment on the same line, which in an
	// unquoted step usually means text was cut off.
	Comment string
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Src) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return &yaml.TypeError{Errors: []string{fmt.Sprintf("line %d: expected a single value (text, number or expression)", n.Line)}}
	}
	*s = Src{Text: n.Value, Line: n.Line, Col: n.Column, Style: n.Style, Set: true, Comment: n.LineComment}
	switch n.Tag {
	case "!!int", "!!float":
		if f, err := strconv.ParseFloat(n.Value, 64); err == nil {
			s.Num = &f
		}
	case "!!bool":
		b := n.Value == "true"
		s.Bool = &b
	}
	return nil
}

// AmountSrc is an amount: one value, or a [low, high] range.
type AmountSrc struct {
	Src
	Hi *Src
}

// UnmarshalYAML accepts a scalar or a two-element sequence.
func (a *AmountSrc) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.SequenceNode {
		if len(n.Content) != 2 {
			return &yaml.TypeError{Errors: []string{fmt.Sprintf("line %d: a range amount needs exactly two values, like [1, 2]", n.Line)}}
		}
		if err := a.Src.UnmarshalYAML(n.Content[0]); err != nil {
			return err
		}
		a.Hi = &Src{}
		return a.Hi.UnmarshalYAML(n.Content[1])
	}
	return a.Src.UnmarshalYAML(n)
}

// at returns the source position of a byte offset within the scalar.
func (s Src) at(file string, offset int) Loc {
	switch s.Style {
	case yaml.DoubleQuotedStyle, yaml.SingleQuotedStyle:
		return Loc{file, s.Line, s.Col + 1 + offset}
	case yaml.LiteralStyle, yaml.FoldedStyle:
		return Loc{file, s.Line + 1, 0}
	default:
		return Loc{file, s.Line, s.Col + offset}
	}
}

// File is a recipe file as written.
type File struct {
	Title        string            `yaml:"title"`
	Slug         string            `yaml:"slug"`
	Description  string            `yaml:"description"`
	Image        string            `yaml:"image"`
	ImageAlt     string            `yaml:"image_alt"`
	Category     string            `yaml:"category"`
	Tags         []string          `yaml:"tags"`
	Cuisine      string            `yaml:"cuisine"`
	Author       string            `yaml:"author"`
	Source       *Source           `yaml:"source"`
	Date         string            `yaml:"date"`
	Draft        bool              `yaml:"draft"`
	Servings     Servings          `yaml:"servings"`
	Scaling      Src               `yaml:"scaling"`
	Options      []Option          `yaml:"options"`
	Vars         yaml.Node         `yaml:"vars"`
	Times        yaml.Node         `yaml:"times"`
	Equipment    []Equipment       `yaml:"equipment"`
	Ingredients  []IngEntry        `yaml:"ingredients"`
	Steps        []StepEntry       `yaml:"steps"`
	Notes        []Note            `yaml:"notes"`
	IngredientDB map[string]DBYAML `yaml:"ingredient_db"`
}

// Source credits where a recipe came from.
type Source struct {
	Name string `yaml:"name" json:"name,omitempty"`
	URL  string `yaml:"url" json:"url,omitempty"`
}

// Servings configures the servings control.
type Servings struct {
	Default float64 `yaml:"default"`
	Min     float64 `yaml:"min"`
	Max     float64 `yaml:"max"`
	Step    float64 `yaml:"step"`
	Unit    string  `yaml:"unit"`
	Plural  string  `yaml:"plural"`
	Line    int     `yaml:"-"`
}

// UnmarshalYAML accepts "servings: 4" or a mapping.
func (s *Servings) UnmarshalYAML(n *yaml.Node) error {
	s.Line = n.Line
	if n.Kind == yaml.ScalarNode {
		f, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return &yaml.TypeError{Errors: []string{fmt.Sprintf("line %d: servings must be a number or a mapping", n.Line)}}
		}
		s.Default = f
		return nil
	}
	type plain Servings
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	p.Line = n.Line
	*s = Servings(p)
	return nil
}

// Option is a reader-selectable customisation.
type Option struct {
	ID      string    `yaml:"id"`
	Type    string    `yaml:"type"`
	Label   string    `yaml:"label"`
	Help    string    `yaml:"help"`
	Default yaml.Node `yaml:"default"`
	Choices []Choice  `yaml:"choices"`
	Min     *float64  `yaml:"min"`
	Max     *float64  `yaml:"max"`
	Step    *float64  `yaml:"step"`
	Unit    string    `yaml:"unit"`
	When    Src       `yaml:"when"`
	Line    int       `yaml:"-"`
	Col     int       `yaml:"-"`
}

// UnmarshalYAML records the position.
func (o *Option) UnmarshalYAML(n *yaml.Node) error {
	type plain Option
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	p.Line, p.Col = n.Line, n.Column
	*o = Option(p)
	return nil
}

// Choice is one value of a choice option.
type Choice struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
}

// UnmarshalYAML accepts a plain string or {id, label}.
func (c *Choice) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		c.Label = n.Value
		return nil
	}
	type plain Choice
	return n.Decode((*plain)(c))
}

// Equipment is a tool needed for the recipe.
type Equipment struct {
	ID   string `yaml:"id"`
	Name Src    `yaml:"name"`
	Note Src    `yaml:"note"`
	When Src    `yaml:"when"`
	Line int    `yaml:"-"`
}

// UnmarshalYAML accepts a plain name or a mapping.
func (e *Equipment) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*e = Equipment{Line: n.Line}
		return e.Name.UnmarshalYAML(n)
	}
	type plain Equipment
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	p.Line = n.Line
	*e = Equipment(p)
	return nil
}

// IngEntry is either an ingredient or a section of ingredients.
type IngEntry struct {
	Section *IngSection
	Item    *IngItem
}

// IngSection groups ingredients.
type IngSection struct {
	Section string     `yaml:"section"`
	When    Src        `yaml:"when"`
	Items   []IngEntry `yaml:"items"`
	Line    int        `yaml:"-"`
}

// IngItem is a single ingredient.
type IngItem struct {
	ID          string    `yaml:"id"`
	Ingredient  string    `yaml:"ingredient"`
	Name        string    `yaml:"name"`
	Plural      string    `yaml:"plural"`
	Amount      AmountSrc `yaml:"amount"`
	Unit        string    `yaml:"unit"`
	Scale       *bool     `yaml:"scale"`
	FixedUnit   bool      `yaml:"fixed_unit"`
	Note        Src       `yaml:"note"`
	When        Src       `yaml:"when"`
	Optional    bool      `yaml:"optional"`
	Default     bool      `yaml:"default"`
	OptionLabel string    `yaml:"option_label"`
	Recipe      string    `yaml:"recipe"`
	Split       yaml.Node `yaml:"split"`
	With        yaml.Node `yaml:"with"`
	Line        Src       `yaml:"-"` // shorthand text, e.g. "2 cups flour, sifted"
	Pos         int       `yaml:"-"`
}

// UnmarshalYAML accepts shorthand text, a section, or an ingredient mapping.
func (e *IngEntry) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		it := &IngItem{Pos: n.Line}
		if err := it.Line.UnmarshalYAML(n); err != nil {
			return err
		}
		e.Item = it
		return nil
	}
	if n.Kind == yaml.MappingNode && (hasKey(n, "section") || hasKey(n, "items")) {
		var s IngSection
		if err := n.Decode(&s); err != nil {
			return err
		}
		s.Line = n.Line
		e.Section = &s
		return nil
	}
	type plain IngItem
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	p.Pos = n.Line
	it := IngItem(p)
	e.Item = &it
	return nil
}

// StepEntry is either a step or a section of steps.
type StepEntry struct {
	Section *StepSection
	Step    *Step
}

// StepSection groups steps.
type StepSection struct {
	Section string      `yaml:"section"`
	When    Src         `yaml:"when"`
	Steps   []StepEntry `yaml:"steps"`
	Line    int         `yaml:"-"`
}

// Step is one instruction.
type Step struct {
	Text  Src    `yaml:"text"`
	When  Src    `yaml:"when"`
	Note  Src    `yaml:"note"`
	Image string `yaml:"image"`
	Line  int    `yaml:"-"`
}

// UnmarshalYAML accepts plain text, a section, or a step mapping.
func (e *StepEntry) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		st := &Step{Line: n.Line}
		if err := st.Text.UnmarshalYAML(n); err != nil {
			return err
		}
		e.Step = st
		return nil
	}
	if n.Kind == yaml.MappingNode && (hasKey(n, "section") || hasKey(n, "steps")) {
		var s StepSection
		if err := n.Decode(&s); err != nil {
			return err
		}
		s.Line = n.Line
		e.Section = &s
		return nil
	}
	type plain Step
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	p.Line = n.Line
	st := Step(p)
	e.Step = &st
	return nil
}

// Note is a callout shown with the recipe.
type Note struct {
	Text Src    `yaml:"text"`
	When Src    `yaml:"when"`
	Kind string `yaml:"kind"`
	Line int    `yaml:"-"`
}

// UnmarshalYAML accepts plain text or a mapping.
func (nt *Note) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*nt = Note{Line: n.Line}
		return nt.Text.UnmarshalYAML(n)
	}
	type plain Note
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	p.Line = n.Line
	*nt = Note(p)
	return nil
}

func hasKey(n *yaml.Node, key string) bool {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return true
		}
	}
	return false
}

// orderedPairs returns key/value nodes of a mapping in source order.
func orderedPairs(n *yaml.Node) [][2]*yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	out := make([][2]*yaml.Node, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		out = append(out, [2]*yaml.Node{n.Content[i], n.Content[i+1]})
	}
	return out
}
