// Package runtime renders a compiled recipe for a given state (servings,
// options, unit system) into a display model. The TypeScript runtime in
// web/src/runtime mirrors this file; testdata/conformance keeps them in sync.
package runtime

import (
	"fmt"
	"math"
	"strings"

	"github.com/RashKash103/get-cooked/internal/expr"
	"github.com/RashKash103/get-cooked/internal/recipe"
	"github.com/RashKash103/get-cooked/internal/tmpl"
	"github.com/RashKash103/get-cooked/internal/units"
)

// State is the reader's selection.
type State struct {
	Servings float64        `json:"servings,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
	System   string         `json:"system,omitempty"`
}

// Model is the rendered recipe.
type Model struct {
	Servings    float64        `json:"servings"`
	Yield       string         `json:"yield"`
	Scale       float64        `json:"scale"`
	System      string         `json:"system"`
	Options     []string       `json:"options"`
	Times       []TimeM        `json:"times"`
	Equipment   []EquipM       `json:"equipment"`
	Ingredients []IngSectionM  `json:"ingredients"`
	Steps       []StepSectionM `json:"steps"`
	Notes       []NoteM        `json:"notes"`
}

// TimeM is a rendered time.
type TimeM struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Minutes float64 `json:"minutes"`
	Text    string  `json:"text"`
}

// EquipM is rendered equipment.
type EquipM struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Note string `json:"note"`
}

// IngSectionM is a rendered ingredient section.
type IngSectionM struct {
	Title string `json:"title"`
	Items []IngM `json:"items"`
}

// IngM is a rendered ingredient.
type IngM struct {
	ID       string     `json:"id"`
	Q        *units.Qty `json:"q"`
	Name     string     `json:"name"`
	Note     string     `json:"note"`
	Optional bool       `json:"optional"`
}

// StepSectionM is a rendered step section.
type StepSectionM struct {
	Title string  `json:"title"`
	Items []StepM `json:"items"`
}

// StepM is a rendered step.
type StepM struct {
	N     int    `json:"n"`
	Segs  []Seg  `json:"segs"`
	Note  string `json:"note"`
	Image string `json:"image"`
}

// Seg is a piece of rendered step text.
type Seg struct {
	T   string  `json:"t"` // text | ing | eq | timer
	V   string  `json:"v"`
	ID  string  `json:"id"`
	Q   string  `json:"q"`
	Min float64 `json:"min"`
}

// NoteM is a rendered note.
type NoteM struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Error is a render error with the source location of the failing element.
type Error struct {
	Loc recipe.Loc
	Msg string
}

func (e *Error) Error() string { return e.Msg }

type rctx struct {
	c      *recipe.Compiled
	system string
	env    map[string]expr.Value
	ings   map[string]*active // active ingredients by id
	equip  map[string]string  // active equipment names by id
}

type active struct {
	ic    *recipe.IngC
	value float64
	has   bool
}

func fail(l recipe.Loc, format string, args ...any) error {
	return &Error{Loc: l, Msg: fmt.Sprintf(format, args...)}
}

// Env resolves the expression environment for a state.
func Env(c *recipe.Compiled, st State) (map[string]expr.Value, float64, string, error) {
	servings := st.Servings
	if servings <= 0 {
		servings = c.Servings.Default
	}
	servings = math.Min(math.Max(servings, c.Servings.Min), c.Servings.Max)
	system := st.System
	if system != units.US && system != units.Metric && system != units.Original {
		system = c.System
	}
	env := map[string]expr.Value{
		"servings":      expr.Num(servings),
		"base_servings": expr.Num(c.Servings.Default),
		"scale":         expr.Num(servings / c.Servings.Default),
		"metric":        expr.Bool(system == units.Metric),
	}
	for _, o := range c.Options {
		env[o.ID] = optionValue(o, st.Options[o.ID])
	}
	if c.Scale != nil {
		s, err := c.Scale.EvalNum(expr.MapEnv(env))
		if err != nil {
			return nil, 0, "", &Error{Msg: "scaling: " + err.Error()}
		}
		if s <= 0 {
			return nil, 0, "", &Error{Msg: fmt.Sprintf("scaling: scale must be positive, got %s", expr.FormatNumber(s))}
		}
		env["scale"] = expr.Num(s)
	}
	for _, v := range c.Vars {
		val, err := v.E.Eval(expr.MapEnv(env))
		if err != nil {
			return nil, 0, "", fail(v.L, "var %s: %v", v.ID, err)
		}
		env[v.ID] = val
	}
	return env, servings, system, nil
}

func optionValue(o recipe.OptionC, raw any) expr.Value {
	switch o.Type {
	case "toggle":
		if b, ok := raw.(bool); ok {
			return expr.Bool(b)
		}
		b, _ := o.Default.(bool)
		return expr.Bool(b)
	case "choice":
		if s, ok := raw.(string); ok {
			for _, ch := range o.Choices {
				if ch.ID == s {
					return expr.Str(s)
				}
			}
		}
		s, _ := o.Default.(string)
		return expr.Str(s)
	default:
		def, _ := o.Default.(float64)
		v := def
		if f, ok := raw.(float64); ok {
			v = f
		}
		if o.Min != nil {
			v = math.Max(v, *o.Min)
		}
		if o.Max != nil {
			v = math.Min(v, *o.Max)
		}
		return expr.Num(v)
	}
}

func (r *rctx) cond(e *expr.Expr, l recipe.Loc) (bool, error) {
	if e == nil {
		return true, nil
	}
	b, err := e.EvalBool(expr.MapEnv(r.env))
	if err != nil {
		return false, fail(l, "when: %v", err)
	}
	return b, nil
}

// Render renders the recipe for a state.
func Render(c *recipe.Compiled, st State) (*Model, error) {
	env, servings, system, err := Env(c, st)
	if err != nil {
		return nil, err
	}
	r := &rctx{c: c, system: system, env: env, ings: map[string]*active{}, equip: map[string]string{}}
	m := &Model{
		Servings: servings, Scale: expr.RoundTo(env["scale"].Num, 4), System: system,
		Options: []string{}, Times: []TimeM{}, Equipment: []EquipM{}, Ingredients: []IngSectionM{},
		Steps: []StepSectionM{}, Notes: []NoteM{},
	}
	unit := c.Servings.Other
	if servings == 1 {
		unit = c.Servings.One
	}
	m.Yield = c.Units.FormatFraction(servings) + " " + unit

	for _, o := range c.Options {
		ok, err := r.cond(o.When, o.L)
		if err != nil {
			return nil, err
		}
		if ok {
			m.Options = append(m.Options, o.ID)
		}
	}
	for _, t := range c.Times {
		v, err := t.E.EvalNum(expr.MapEnv(env))
		if err != nil {
			return nil, fail(t.L, "times.%s: %v", t.ID, err)
		}
		if v < 0 {
			return nil, fail(t.L, "times.%s is negative (%s)", t.ID, expr.FormatNumber(v))
		}
		m.Times = append(m.Times, TimeM{ID: t.ID, Label: t.Label, Minutes: expr.RoundTo(v, 2), Text: units.Duration(v)})
	}

	// Resolve active ingredients first so steps can reference them.
	type pendingSection struct {
		title string
		items []*active
	}
	var pending []pendingSection
	for si := range c.Ingredients {
		s := &c.Ingredients[si]
		ok, err := r.cond(s.When, s.L)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		ps := pendingSection{title: s.Title}
		for ii := range s.Items {
			ic := &s.Items[ii]
			ok, err := r.cond(ic.When, ic.L)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			a := &active{ic: ic}
			if ic.Amount != nil {
				v, err := r.amount(ic)
				if err != nil {
					return nil, err
				}
				if v == 0 {
					continue
				}
				a.value, a.has = v, true
			}
			if _, dup := r.ings[ic.ID]; dup {
				return nil, fail(ic.L, "ingredient %q is included twice; its `when:` conditions overlap", ic.ID)
			}
			r.ings[ic.ID] = a
			ps.items = append(ps.items, a)
		}
		pending = append(pending, ps)
	}
	for _, e := range c.Equipment {
		ok, err := r.cond(e.When, e.L)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if _, dup := r.equip[e.ID]; dup {
			return nil, fail(e.L, "equipment %q is included twice; its `when:` conditions overlap", e.ID)
		}
		r.equip[e.ID] = "" // reserve id before rendering names that may reference others
		name, err := r.flat(e.Name, e.L)
		if err != nil {
			return nil, err
		}
		r.equip[e.ID] = name
		note, err := r.flat(e.Note, e.L)
		if err != nil {
			return nil, err
		}
		m.Equipment = append(m.Equipment, EquipM{ID: e.ID, Name: name, Note: note})
	}
	for _, ps := range pending {
		sec := IngSectionM{Title: ps.title, Items: []IngM{}}
		for _, a := range ps.items {
			im := IngM{ID: a.ic.ID, Optional: a.ic.Optional}
			if a.has {
				q := r.qty(a.ic, a.value)
				im.Q = &q
			}
			im.Name = r.name(a.ic, im.Q)
			note, err := r.flat(a.ic.Note, a.ic.L)
			if err != nil {
				return nil, err
			}
			im.Note = note
			sec.Items = append(sec.Items, im)
		}
		if len(sec.Items) > 0 {
			m.Ingredients = append(m.Ingredients, sec)
		}
	}
	n := 0
	for _, s := range c.Steps {
		ok, err := r.cond(s.When, s.L)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		sec := StepSectionM{Title: s.Title, Items: []StepM{}}
		for _, st := range s.Items {
			ok, err := r.cond(st.When, st.L)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			segs, err := r.segs(st.Text, st.L, nil)
			if err != nil {
				return nil, err
			}
			note, err := r.flat(st.Note, st.L)
			if err != nil {
				return nil, err
			}
			n++
			sec.Items = append(sec.Items, StepM{N: n, Segs: merge(segs), Note: note, Image: st.Image})
		}
		if len(sec.Items) > 0 {
			m.Steps = append(m.Steps, sec)
		}
	}
	for _, nt := range c.Notes {
		ok, err := r.cond(nt.When, nt.L)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		text, err := r.flat(nt.Text, nt.L)
		if err != nil {
			return nil, err
		}
		m.Notes = append(m.Notes, NoteM{Kind: nt.Kind, Text: text})
	}
	return m, nil
}

func (r *rctx) amount(ic *recipe.IngC) (float64, error) {
	var v float64
	switch {
	case ic.Amount.E != nil:
		x, err := ic.Amount.E.EvalNum(expr.MapEnv(r.env))
		if err != nil {
			return 0, fail(ic.L, "amount of %q: %v", ic.ID, err)
		}
		v = x
	case ic.Amount.S:
		v = *ic.Amount.N * r.env["scale"].Num
	default:
		v = *ic.Amount.N
	}
	if v < 0 {
		return 0, fail(ic.L, "amount of %q is negative (%s)", ic.ID, expr.FormatNumber(v))
	}
	return v, nil
}

func (r *rctx) qty(ic *recipe.IngC, v float64) units.Qty {
	unit := ic.Unit
	if unit == "" {
		unit = "each"
	}
	var info *units.Ing
	if ic.Ref != "" {
		info = r.c.Ings[ic.Ref]
	}
	return r.c.Units.Format(v, unit, r.system, info, ic.Fixed)
}

func (r *rctx) name(ic *recipe.IngC, q *units.Qty) string {
	if q != nil && q.ID == "each" && q.Value > 1 && ic.Plural != "" {
		return ic.Plural
	}
	return ic.Name
}

func merge(segs []Seg) []Seg {
	out := []Seg{}
	for _, s := range segs {
		if s.T == "text" {
			if s.V == "" {
				continue
			}
			if n := len(out); n > 0 && out[n-1].T == "text" {
				out[n-1].V += s.V
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// flat renders a template to plain text.
func (r *rctx) flat(t tmpl.Template, l recipe.Loc) (string, error) {
	if len(t) == 0 {
		return "", nil
	}
	segs, err := r.segs(t, l, nil)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, s := range segs {
		if s.T == "ing" && s.Q != "" {
			sb.WriteString(s.Q + " ")
		}
		sb.WriteString(s.V)
	}
	return sb.String(), nil
}

func (r *rctx) segs(t tmpl.Template, l recipe.Loc, out []Seg) ([]Seg, error) {
	env := expr.MapEnv(r.env)
	for _, s := range t {
		switch s.Kind {
		case tmpl.KText:
			out = append(out, Seg{T: "text", V: s.Text})
		case tmpl.KExpr:
			v, err := s.Expr.Eval(env)
			if err != nil {
				return nil, fail(l, "%v", err)
			}
			text, err := r.format(v, s.Fmt, s.Args)
			if err != nil {
				return nil, fail(l, "%v", err)
			}
			out = append(out, Seg{T: "text", V: text})
		case tmpl.KIng:
			a, ok := r.ings[s.ID]
			if !ok {
				return nil, fail(l, "@%s is referenced but not included for this selection; wrap the reference in {if …}{end} or adjust `when:`", s.ID)
			}
			seg := Seg{T: "ing", ID: s.ID}
			var q *units.Qty
			if a.has && !s.NameOnly {
				v := a.value
				if s.Expr != nil {
					p, err := s.Expr.EvalNum(env)
					if err != nil {
						return nil, fail(l, "@%s portion: %v", s.ID, err)
					}
					if p <= 0 {
						return nil, fail(l, "@%s portion must be positive", s.ID)
					}
					v *= p
				}
				qq := r.qty(a.ic, v)
				q = &qq
				seg.Q = qq.String()
			}
			seg.V = r.name(a.ic, q)
			out = append(out, seg)
		case tmpl.KEquip:
			name, ok := r.equip[s.ID]
			if !ok {
				return nil, fail(l, "#%s is referenced but not included for this selection", s.ID)
			}
			out = append(out, Seg{T: "eq", ID: s.ID, V: name})
		case tmpl.KTimer:
			v, err := s.Expr.EvalNum(env)
			if err != nil {
				return nil, fail(l, "timer: %v", err)
			}
			if v <= 0 {
				return nil, fail(l, "timer must be positive, got %s", expr.FormatNumber(v))
			}
			out = append(out, Seg{T: "timer", V: units.Duration(v), Min: expr.RoundTo(v, 2)})
		case tmpl.KIf:
			matched := false
			for _, b := range s.Branches {
				c, err := b.Cond.EvalBool(env)
				if err != nil {
					return nil, fail(l, "{if}: %v", err)
				}
				if c {
					var err error
					if out, err = r.segs(b.Body, l, out); err != nil {
						return nil, err
					}
					matched = true
					break
				}
			}
			if !matched {
				var err error
				if out, err = r.segs(s.Else, l, out); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, nil
}

var fracRule = units.Rule{Denoms: []int{2, 3, 4, 8}}

func (r *rctx) format(v expr.Value, f string, args []string) (string, error) {
	if f == "" {
		return v.String(), nil
	}
	if v.Kind != expr.KNum {
		return "", fmt.Errorf("formatter %q needs a number, got %s %q", f, v.Kind, v.String())
	}
	x := v.Num
	switch f {
	case "frac":
		return r.c.Units.FormatFraction(fracRule.Round(x)), nil
	case "int":
		return expr.FormatNumber(math.Round(x)), nil
	case "duration":
		return units.Duration(x), nil
	case "temp":
		return units.Temp(x, false, r.system), nil
	case "temp_c":
		return units.Temp(x, true, r.system), nil
	case "len":
		return units.Length(x, false, r.system), nil
	case "len_cm":
		return units.Length(x, true, r.system), nil
	case "qty":
		return r.c.Units.Format(x, args[0], r.system, nil, false).String(), nil
	}
	return "", fmt.Errorf("unknown formatter %q", f)
}
