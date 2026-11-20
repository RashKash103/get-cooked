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
	// Image is the matching image variant (nil: use the recipe's default image).
	Image *ImageM `json:"image"`
	// Makes is how much the recipe makes at these servings (nil: not given).
	Makes *units.Qty `json:"makes"`
}

// ImageM is the image variant chosen for the state.
type ImageM struct {
	Src    string `json:"src"`
	SrcSet string `json:"srcset"`
	Alt    string `json:"alt"`
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
	J        string     `json:"j"` // text between quantity and name
	Name     string     `json:"name"`
	Note     string     `json:"note"`
	Optional bool       `json:"optional"`
	Link     *LinkM     `json:"link"`
	Uses     []UseM     `json:"uses"`
	Swap     string     `json:"swap,omitempty"` // the swap_<id> option when it has substitutes
}

// UseM is the amount of an ingredient used by one step, listed when an
// ingredient is divided between several steps.
type UseM struct {
	N int    `json:"n"`
	Q string `json:"q"`
}

// LinkM points at another recipe with the servings and options to open it with.
type LinkM struct {
	Slug     string         `json:"slug"`
	Servings float64        `json:"servings"`
	Options  map[string]any `json:"options"`
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
	T    string  `json:"t"` // text | ing | eq | timer
	V    string  `json:"v"`
	ID   string  `json:"id"`
	Q    string  `json:"q"`
	J    string  `json:"j"` // text between quantity and name (ing)
	Min  float64 `json:"min"`
	Max  float64 `json:"max"` // timer: upper end of a range (0 when not a range)
	Link *LinkM  `json:"link"`
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
	step   int                // step being rendered (0 outside steps)
	uses   map[string][]UseM
}

type active struct {
	ic    *recipe.IngC
	value float64
	hi    float64 // upper end of a range (0 when not a range)
	has   bool
	parts map[string]float64
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
	r := &rctx{c: c, system: system, env: env, ings: map[string]*active{}, equip: map[string]string{}, uses: map[string][]UseM{}}
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
	if mk := c.Makes; mk != nil {
		q := c.Units.Format(mk.Amount*env["scale"].Num, mk.Unit, system, c.Ings[mk.Key], false)
		m.Makes = &q
	}

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
		m.Times = append(m.Times, TimeM{ID: t.ID, Label: t.Label, Minutes: expr.RoundTo(v, 2), Text: r.c.Units.Duration(v)})
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
				v, err := r.amount(ic, *ic.Amount, ic.ID)
				if err != nil {
					return nil, err
				}
				if v == 0 {
					continue
				}
				a.value, a.has = v, true
				if ic.Amount.Hi != nil {
					h, err := r.amount(ic, *ic.Amount.Hi, ic.ID)
					if err != nil {
						return nil, err
					}
					if h < v {
						return nil, fail(ic.L, "range of %q goes down: %s to %s", ic.ID, expr.FormatNumber(v), expr.FormatNumber(h))
					}
					a.hi = h
				}
			}
			if len(ic.Split) > 0 {
				a.parts = map[string]float64{}
				for _, sp := range ic.Split {
					v, err := r.amount(ic, sp.A, ic.ID+"."+sp.ID)
					if err != nil {
						return nil, err
					}
					a.parts[sp.ID] = v
					a.value += v
				}
				if a.value == 0 {
					continue
				}
				a.has = true
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
			im := IngM{ID: a.ic.ID, Optional: a.ic.Optional, Uses: []UseM{}, Swap: a.ic.Swap}
			if a.has {
				q := r.qtyRange(a.ic, a.value, a.hi)
				im.Q = &q
				if im.Link, err = r.link(a.ic, a.value); err != nil {
					return nil, err
				}
			}
			im.Name = r.name(a.ic, im.Q)
			im.J = r.c.Units.Join(im.Q)
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
			n++
			r.step = n
			segs, err := r.segs(st.Text, st.L, nil)
			r.step = 0
			if err != nil {
				return nil, err
			}
			note, err := r.flat(st.Note, st.L)
			if err != nil {
				return nil, err
			}
			sec.Items = append(sec.Items, StepM{N: n, Segs: merge(segs), Note: note, Image: st.Image})
		}
		if len(sec.Items) > 0 {
			m.Steps = append(m.Steps, sec)
		}
	}
	for _, im := range c.Images {
		ok, err := r.cond(im.When, im.L)
		if err != nil {
			return nil, err
		}
		if ok {
			m.Image = &ImageM{Src: im.Src, SrcSet: im.SrcSet, Alt: im.Alt}
			break
		}
	}
	// An ingredient divided between steps lists each step's share.
	for si := range m.Ingredients {
		for ii := range m.Ingredients[si].Items {
			it := &m.Ingredients[si].Items[ii]
			if u := r.uses[it.ID]; len(u) >= 2 {
				it.Uses = u
			}
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

func (r *rctx) amount(ic *recipe.IngC, a recipe.AmountC, what string) (float64, error) {
	var v float64
	switch {
	case a.E != nil:
		x, err := a.E.EvalNum(expr.MapEnv(r.env))
		if err != nil {
			return 0, fail(ic.L, "amount of %q: %v", what, err)
		}
		v = x
	case a.N == nil:
		v = 0
	case a.S:
		v = *a.N * r.env["scale"].Num
	default:
		v = *a.N
	}
	if v < 0 {
		return 0, fail(ic.L, "amount of %q is negative (%s)", what, expr.FormatNumber(v))
	}
	return v, nil
}

var recipeRule = units.Rule{Denoms: []int{2, 3, 4}}

// link builds the link to a referenced recipe: servings are rounded up to
// what the target can make, and `with:` options are evaluated here.
func (r *rctx) link(ic *recipe.IngC, v float64) (*LinkM, error) {
	ref := ic.Recipe
	if ref == nil {
		return nil, nil
	}
	step := ref.Step
	if step <= 0 {
		step = 1
	}
	if ref.Unit != "" {
		// Measured: how many servings make this much.
		unit := ic.Unit
		if unit == "" {
			unit = "each"
		}
		n, ok := r.c.Units.Convert(v, unit, ref.Unit, r.c.Ings[ref.Key])
		if !ok || ref.Per <= 0 {
			v = 0
		} else {
			v = n / ref.Per
		}
	}
	s := math.Ceil(v/step-1e-9) * step
	s = expr.RoundTo(math.Min(math.Max(s, ref.Min), ref.Max), 6)
	l := &LinkM{Slug: ref.Slug, Servings: s, Options: map[string]any{}}
	for _, w := range ref.With {
		val, err := w.E.Eval(expr.MapEnv(r.env))
		if err != nil {
			return nil, fail(ic.L, "with.%s: %v", w.ID, err)
		}
		want := map[string]expr.Kind{"toggle": expr.KBool, "choice": expr.KStr, "number": expr.KNum}[w.Type]
		if w.Type != "" && val.Kind != want {
			return nil, fail(ic.L, "with.%s: option is a %s, but the value is %s %q", w.ID, w.Type, val.Kind, val.String())
		}
		l.Options[w.ID] = val.Interface()
	}
	return l, nil
}

// qtyRange formats an amount, or a range when hi > 0.
func (r *rctx) qtyRange(ic *recipe.IngC, v, hi float64) units.Qty {
	if hi <= 0 || ic.Recipe != nil && ic.Recipe.Unit == "" {
		return r.qty(ic, v)
	}
	unit := ic.Unit
	if unit == "" {
		unit = "each"
	}
	var info *units.Ing
	if ic.Ref != "" {
		info = r.c.Ings[ic.Ref]
	}
	return r.c.Units.FormatRange(v, hi, unit, r.system, info, ic.Fixed)
}

func (r *rctx) qty(ic *recipe.IngC, v float64) units.Qty {
	if ref := ic.Recipe; ref != nil && ref.Unit == "" {
		q := recipeRule.Round(v)
		if q == 0 {
			q = 0.25
		}
		unit := ref.Other
		if q <= 1 {
			unit = ref.One
		}
		return units.Qty{Value: q, Text: r.c.Units.FormatFraction(q), Unit: unit, ID: "recipe"}
	}
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
	if q != nil && q.ID == "each" && q.Max() > 1 && ic.Plural != "" {
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
			sb.WriteString(s.Q + s.J)
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
				if s.Part != "" {
					pv, ok := a.parts[s.Part]
					if !ok {
						return nil, fail(l, "@%s.%s: this variant of %q has no split named %q", s.ID, s.Part, s.ID, s.Part)
					}
					v = pv
				}
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
				h := 0.0
				if s.Part == "" && a.hi > 0 {
					h = a.hi * v / a.value
				}
				qq := r.qtyRange(a.ic, v, h)
				q = &qq
				seg.Q = qq.String()
				seg.J = r.c.Units.Join(q)
				if r.step > 0 && (s.Part != "" || s.Expr != nil) {
					r.uses[s.ID] = append(r.uses[s.ID], UseM{N: r.step, Q: seg.Q})
				}
			}
			if a.has {
				link, err := r.link(a.ic, a.value)
				if err != nil {
					return nil, err
				}
				seg.Link = link
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
			if s.Hi != nil {
				h, err := s.Hi.EvalNum(env)
				if err != nil {
					return nil, fail(l, "timer: %v", err)
				}
				if h <= v {
					return nil, fail(l, "timer range must go up: ~{%s, %s}", expr.FormatNumber(v), expr.FormatNumber(h))
				}
				out = append(out, Seg{T: "timer", V: r.c.Units.DurationRange(v, h), Min: expr.RoundTo(v, 2), Max: expr.RoundTo(h, 2)})
				continue
			}
			out = append(out, Seg{T: "timer", V: r.c.Units.Duration(v), Min: expr.RoundTo(v, 2)})
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
	u := r.c.Units
	if f == "" {
		if v.Kind == expr.KNum {
			return u.L(v.String()), nil
		}
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
		return u.L(expr.FormatNumber(math.Round(x))), nil
	case "duration":
		return r.c.Units.Duration(x), nil
	case "temp":
		return u.L(units.Temp(x, false, r.system)), nil
	case "temp_c":
		return u.L(units.Temp(x, true, r.system)), nil
	case "len":
		return u.L(units.Length(x, false, r.system)), nil
	case "len_cm":
		return u.L(units.Length(x, true, r.system)), nil
	case "qty":
		return r.c.Units.Format(x, args[0], r.system, nil, false).String(), nil
	}
	return "", fmt.Errorf("unknown formatter %q", f)
}
