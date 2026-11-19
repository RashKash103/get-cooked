package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/RashKash103/get-cooked/internal/expr"
	"github.com/RashKash103/get-cooked/internal/i18n"
	"github.com/RashKash103/get-cooked/internal/ingline"
	"github.com/RashKash103/get-cooked/internal/tmpl"
	"github.com/RashKash103/get-cooked/internal/units"
)

// Builtins are identifiers available in every expression.
var Builtins = []string{"servings", "base_servings", "scale", "metric"}

var idRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Compiler compiles recipe files.
type Compiler struct {
	Units  *units.Table
	DB     *DB
	System string // default unit system
	// Catalog provides built-in labels (times, servings, "Add …") in the
	// target language. Nil means English.
	Catalog *i18n.Catalog
	// Tr translates recipe text (English source → target language). It is
	// called for every user-visible string; nil means no translation.
	Tr func(string) string
}

// CompileFile reads and compiles a recipe file.
func (c *Compiler) CompileFile(path string) (*Recipe, Diags) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, Diags{{File: path, Msg: err.Error()}}
	}
	return c.Compile(path, b)
}

var yamlLineRe = regexp.MustCompile(`line (\d+):?\s*`)

type cctx struct {
	*Compiler
	file      string
	db        *DB
	diags     Diags
	symbols   map[string]bool
	ingIDs    map[string]bool
	eqIDs     map[string]bool
	usedDB    map[string]bool
	optWhen   map[string]Src
	parts     map[string]map[string]bool // ingredient id → split names
	overrides map[string]*units.Ing      // per-recipe ingredient overrides
}

// tr translates user-visible text.
func (cc *cctx) tr(s string) string {
	if s == "" || cc.Tr == nil {
		return s
	}
	return cc.Tr(s)
}

func (cc *cctx) errAt(l Loc, format string, args ...any) {
	cc.diags = append(cc.diags, l.Diag(format, args...))
}

func (cc *cctx) loc(line, col int) Loc { return Loc{cc.file, line, col} }

// Compile compiles recipe source. path is used for diagnostics and the slug.
func (c *Compiler) Compile(path string, src []byte) (*Recipe, Diags) {
	cc := &cctx{Compiler: c, file: path, db: c.DB, symbols: map[string]bool{}, ingIDs: map[string]bool{}, eqIDs: map[string]bool{}, usedDB: map[string]bool{}, optWhen: map[string]Src{}, parts: map[string]map[string]bool{}, overrides: map[string]*units.Ing{}}
	if cc.db == nil {
		cc.db = NewDB()
	}
	if cc.Catalog == nil {
		cc.Compiler = &Compiler{Units: c.Units, DB: c.DB, System: c.System, Catalog: i18n.English(), Tr: c.Tr}
	}
	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		d := Diag{File: path, Msg: err.Error()}
		if m := yamlLineRe.FindStringSubmatch(err.Error()); m != nil {
			d.Line, _ = strconv.Atoi(m[1])
			d.Msg = strings.TrimPrefix(yamlLineRe.ReplaceAllString(err.Error(), ""), "yaml: ")
		}
		return nil, Diags{d}
	}
	if len(root.Content) == 0 {
		return nil, Diags{{File: path, Msg: "empty recipe file"}}
	}
	if ds := ValidateSchema("recipe", path, &root); ds.HasErrors() {
		return nil, ds
	}
	var f File
	if err := root.Content[0].Decode(&f); err != nil {
		return nil, Diags{{File: path, Msg: err.Error()}}
	}
	r := &Recipe{
		Path: path, Slug: f.Slug, Title: cc.tr(f.Title), Description: cc.tr(strings.TrimSpace(f.Description)),
		Image: f.Image, ImageAlt: cc.tr(f.ImageAlt), Category: cc.tr(f.Category), Cuisine: cc.tr(f.Cuisine),
		Tags: trAll(cc, f.Tags), Author: f.Author, Source: f.Source, Date: f.Date, Draft: f.Draft,
		IngCategories: map[string]string{},
	}
	if r.Slug == "" {
		base := filepath.Base(path)
		r.Slug = Slugify(strings.TrimSuffix(strings.TrimSuffix(base, filepath.Ext(base)), ".recipe"), "-")
	}
	if len(f.IngredientDB) > 0 {
		cc.db = cc.db.Clone()
		keys := make([]string, 0, len(f.IngredientDB))
		for k := range f.IngredientDB {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			cc.diags = append(cc.diags, cc.db.Add(k, f.IngredientDB[k], c.Units, cc.loc(0, 0))...)
		}
	}
	comp := &Compiled{V: Version, System: c.System, Units: c.Units, Ings: map[string]*units.Ing{}}
	if comp.System == "" {
		comp.System = units.Original
	}
	cc.servings(&f, comp)

	// Pass 1: ingredient identities (needed for options and @refs).
	items := cc.collectIngredients(&f)
	for _, s := range Builtins {
		cc.symbols[s] = true
	}
	cc.options(&f, comp, items)
	vars := cc.declareVars(&f)

	cc.equipmentIDs(&f)

	// Pass 2: expressions and templates.
	cc.vars(vars, comp)
	if f.Scaling.Set {
		comp.Scale = cc.expr(f.Scaling, "scaling")
	}
	cc.times(&f, comp)
	cc.equipment(&f, comp)
	cc.ingredients(comp, items, r)
	cc.steps(&f, comp)
	for _, n := range f.Notes {
		kind := n.Kind
		if kind == "" {
			kind = "info"
		}
		comp.Notes = append(comp.Notes, NoteC{When: cc.when(n.When), Kind: kind, Text: cc.template(n.Text), L: cc.loc(n.Line, 0)})
	}
	for _, v := range f.Images {
		l := cc.loc(v.Line, 0)
		if !v.When.Set {
			cc.errAt(l, "image variants need a `when:` condition (the default image is `image:`)")
			continue
		}
		comp.Images = append(comp.Images, ImageC{When: cc.when(v.When), Src: v.Src, Alt: cc.tr(v.Alt), L: l})
	}
	for k := range cc.usedDB {
		comp.Ings[k] = cc.db.Entries[k].Info
	}
	for k, info := range cc.overrides {
		comp.Ings[k] = info
	}
	r.C = comp
	return r, cc.diags
}

func trAll(cc *cctx, list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = cc.tr(s)
	}
	return out
}

// Slugify lowercases s and joins alphanumeric runs with sep.
func Slugify(s, sep string) string {
	var sb strings.Builder
	pending := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if pending && sb.Len() > 0 {
				sb.WriteString(sep)
			}
			pending = false
			sb.WriteRune(r)
			continue
		}
		switch r {
		case 'é', 'è', 'ê':
			r = 'e'
		case 'á', 'à', 'â', 'ä':
			r = 'a'
		case 'ñ':
			r = 'n'
		case 'ó', 'ö', 'ô':
			r = 'o'
		case 'ú', 'ü':
			r = 'u'
		case 'í':
			r = 'i'
		default:
			pending = true
			continue
		}
		if pending && sb.Len() > 0 {
			sb.WriteString(sep)
		}
		pending = false
		sb.WriteRune(r)
	}
	return sb.String()
}

func identFrom(s string) string {
	id := Slugify(s, "_")
	if id == "" {
		return ""
	}
	if id[0] >= '0' && id[0] <= '9' {
		id = "x_" + id
	}
	return id
}

func pluralize(s string) string {
	switch {
	case s == "":
		return s
	case strings.HasSuffix(s, "y") && !strings.HasSuffix(s, "ey") && !strings.HasSuffix(s, "ay") && !strings.HasSuffix(s, "oy"):
		return s[:len(s)-1] + "ies"
	case strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") || strings.HasSuffix(s, "ch") || strings.HasSuffix(s, "sh"):
		return s + "es"
	}
	return s + "s"
}

func (cc *cctx) servings(f *File, comp *Compiled) {
	s := f.Servings
	l := cc.loc(s.Line, 0)
	sc := ServingsC{Default: s.Default, Min: s.Min, Max: s.Max, Step: s.Step, One: cc.tr(s.Unit), Other: cc.tr(s.Plural)}
	if sc.Step == 0 {
		sc.Step = 1
		if sc.Default < 1 {
			sc.Step = sc.Default
		}
	}
	if sc.Min == 0 {
		sc.Min = sc.Step
	}
	if sc.Max == 0 {
		sc.Max = max(sc.Default*4, sc.Min)
	}
	if sc.One == "" {
		sc.One, sc.Other = cc.Catalog.Servings.One, cc.Catalog.Servings.Other
	}
	if sc.Other == "" {
		sc.Other = sc.One
		if cc.Catalog.Lang == i18n.Default {
			sc.Other = pluralize(sc.One)
		}
	}
	if sc.Default < sc.Min || sc.Default > sc.Max {
		cc.errAt(l, "servings.default (%v) must be between min (%v) and max (%v)", sc.Default, sc.Min, sc.Max)
	}
	comp.Servings = sc
}

type ingDraft struct {
	item  *IngItem
	id    string
	ref   string
	name  string
	plur  string
	unit  string
	amt   *AmountC
	loc   Loc
	short ingline.Line
}

type sectionDraft struct {
	sec   *IngSection
	items []*ingDraft
}

func (cc *cctx) collectIngredients(f *File) []*sectionDraft {
	var sections []*sectionDraft
	var implicit *sectionDraft
	add := func(sd *sectionDraft, it *IngItem) {
		d := cc.draftIngredient(it)
		if d != nil {
			sd.items = append(sd.items, d)
		}
	}
	for _, e := range f.Ingredients {
		if e.Section != nil {
			implicit = nil
			sd := &sectionDraft{sec: e.Section}
			sections = append(sections, sd)
			for _, sub := range e.Section.Items {
				if sub.Section != nil {
					cc.errAt(cc.loc(sub.Section.Line, 0), "ingredient sections cannot be nested")
					continue
				}
				add(sd, sub.Item)
			}
			continue
		}
		if implicit == nil {
			implicit = &sectionDraft{}
			sections = append(sections, implicit)
		}
		add(implicit, e.Item)
	}
	// Duplicate ids are only allowed as conditional variants.
	seen := map[string]*ingDraft{}
	for _, sd := range sections {
		for _, d := range sd.items {
			if prev, ok := seen[d.id]; ok && (!d.item.When.Set || !prev.item.When.Set) && !d.item.Optional && !prev.item.Optional {
				cc.errAt(d.loc, "duplicate ingredient id %q (first at line %d); give each variant a `when:` condition or a distinct `id:`", d.id, prev.loc.Line)
			}
			seen[d.id] = d
			cc.ingIDs[d.id] = true
			for _, kv := range orderedPairs(&d.item.Split) {
				if cc.parts[d.id] == nil {
					cc.parts[d.id] = map[string]bool{}
				}
				cc.parts[d.id][kv[0].Value] = true
			}
		}
	}
	return sections
}

func (cc *cctx) draftIngredient(it *IngItem) *ingDraft {
	d := &ingDraft{item: it, loc: cc.loc(it.Pos, 0)}
	var entry *DBEntry
	if it.Line.Set {
		l := ingline.Parse(it.Line.Text, cc.Units)
		d.short = l
		d.name = l.Name
		d.unit = l.Unit
		entry = cc.db.Match(l.Name)
		if l.Amount > 0 {
			a := l.Amount
			d.amt = &AmountC{N: &a, S: true}
			if l.AmountMax > l.Amount {
				h := l.AmountMax
				d.amt.Hi = &AmountC{N: &h, S: true}
			}
		}
		if l.Unit == "" && l.Amount > 1 {
			d.plur = l.Name
			d.name = ingline.Singular(l.Name)
		}
		d.id = identFrom(d.name)
		d.name, d.plur = cc.tr(d.name), cc.tr(d.plur)
		if l.Note != "" {
			it.Note = Src{Text: l.Note, Set: true, Line: it.Line.Line, Col: it.Line.Col}
		}
	} else {
		if it.Ingredient != "" {
			entry = cc.db.Match(it.Ingredient)
			if entry == nil {
				msg := fmt.Sprintf("unknown ingredient %q (not in the ingredient database)", it.Ingredient)
				if s := suggest(it.Ingredient, cc.db.Keys()); s != "" {
					msg += fmt.Sprintf("; did you mean %q?", s)
				}
				cc.errAt(d.loc, "%s", msg)
			}
		} else if it.Name != "" {
			entry = cc.db.Match(it.Name)
		}
		d.name = cc.tr(it.Name)
		d.plur = cc.tr(it.Plural)
		if entry != nil {
			en, ep := cc.entryName(entry)
			if d.name == "" {
				d.name = en
			}
			if d.plur == "" && it.Name == "" {
				d.plur = ep
			}
		}
		if d.name == "" && it.Recipe == "" {
			d.name = strings.ReplaceAll(it.ID, "_", " ")
		}
		d.id = it.ID
		if d.id == "" {
			switch {
			case it.Name != "":
				d.id = identFrom(it.Name)
			case entry != nil:
				d.id = identFrom(entry.Key)
			case it.Recipe != "":
				d.id = identFrom(it.Recipe)
			}
		}
		if it.Recipe != "" && (it.Ingredient != "" || it.Unit != "") {
			cc.errAt(d.loc, "a recipe reference takes its unit from the linked recipe's servings; remove `ingredient:`/`unit:`")
		}
		if it.Unit != "" {
			x, ok := cc.Units.Lookup(it.Unit)
			if !ok {
				msg := fmt.Sprintf("unknown unit %q", it.Unit)
				if s := suggest(it.Unit, cc.Units.Names()); s != "" {
					msg += fmt.Sprintf(" (did you mean %q?)", s)
				}
				cc.errAt(d.loc, "%s; custom count units can be declared under units.custom in get-cooked.yaml", msg)
			} else {
				d.unit = x.ID
			}
		}
	}
	if entry != nil {
		d.ref = entry.Key
		cc.usedDB[entry.Key] = true
		if en, ep := cc.entryName(entry); d.plur == "" && d.name == en {
			d.plur = ep
		}
	}
	if d.unit == "each" {
		d.unit = ""
	}
	if !idRe.MatchString(d.id) {
		cc.errAt(d.loc, "ingredient id %q must be lowercase letters, digits and underscores, starting with a letter", d.id)
	}
	return d
}

// entryName returns the (translated) name and plural of a DB entry.
func (cc *cctx) entryName(e *DBEntry) (string, string) {
	if n, ok := cc.Catalog.Ingredients[e.Key]; ok && n.Name != "" {
		return n.Name, n.Plural
	}
	return e.Name, e.Plural
}

func (cc *cctx) options(f *File, comp *Compiled, sections []*sectionDraft) {
	ids := map[string]bool{}
	for i := range f.Options {
		o := &f.Options[i]
		l := cc.loc(o.Line, o.Col)
		if ids[o.ID] {
			cc.errAt(l, "duplicate option id %q", o.ID)
		}
		if contains(Builtins, o.ID) {
			cc.errAt(l, "option id %q is reserved", o.ID)
		}
		ids[o.ID] = true
		cc.symbols[o.ID] = true
		if o.When.Set {
			cc.optWhen[o.ID] = o.When
		}
		oc := OptionC{ID: o.ID, Type: o.Type, Label: cc.tr(o.Label), Help: cc.tr(o.Help), Unit: o.Unit, L: l}
		if oc.Label == "" {
			oc.Label = strings.ToUpper(o.ID[:1]) + strings.ReplaceAll(o.ID[1:], "_", " ")
		}
		hasDefault := o.Default.Kind != 0
		switch o.Type {
		case "toggle":
			var b bool
			if hasDefault {
				if err := o.Default.Decode(&b); err != nil || o.Default.Tag != "!!bool" {
					cc.errAt(l, "option %q: default must be true or false", o.ID)
				}
			}
			oc.Default = b
		case "choice":
			if len(o.Choices) < 2 {
				cc.errAt(l, "option %q: a choice needs at least two choices", o.ID)
			}
			seen := map[string]bool{}
			for _, ch := range o.Choices {
				id := ch.ID
				if id == "" {
					id = identFrom(ch.Label)
				}
				label := cc.tr(ch.Label)
				if label == "" {
					label = strings.ReplaceAll(id, "_", " ")
				}
				if seen[id] {
					cc.errAt(l, "option %q: duplicate choice %q", o.ID, id)
				}
				seen[id] = true
				oc.Choices = append(oc.Choices, ChoiceC{ID: id, Label: label})
			}
			if len(oc.Choices) > 0 {
				oc.Default = oc.Choices[0].ID
			}
			if hasDefault {
				if !seen[o.Default.Value] {
					cc.errAt(l, "option %q: default %q is not one of the choices", o.ID, o.Default.Value)
				}
				oc.Default = o.Default.Value
			}
		case "number":
			oc.Min, oc.Max, oc.Step = o.Min, o.Max, o.Step
			if oc.Min == nil || oc.Max == nil {
				cc.errAt(l, "option %q: number options need min and max", o.ID)
				zero := 0.0
				oc.Min, oc.Max = &zero, &zero
			}
			if oc.Step == nil {
				one := 1.0
				oc.Step = &one
			}
			def := *oc.Min
			if hasDefault {
				v, err := strconv.ParseFloat(o.Default.Value, 64)
				if err != nil {
					cc.errAt(l, "option %q: default must be a number", o.ID)
				}
				def = v
			}
			if def < *oc.Min || def > *oc.Max {
				cc.errAt(l, "option %q: default %v is outside %v..%v", o.ID, def, *oc.Min, *oc.Max)
			}
			oc.Default = def
		}
		comp.Options = append(comp.Options, oc)
	}
	// Optional ingredients create toggles.
	for _, sd := range sections {
		for _, d := range sd.items {
			if !d.item.Optional {
				continue
			}
			if ids[d.id] {
				for _, o := range comp.Options {
					if o.ID == d.id && o.Type != "toggle" {
						cc.errAt(d.loc, "optional ingredient %q conflicts with %s option of the same id", d.id, o.Type)
					}
				}
				continue
			}
			ids[d.id] = true
			cc.symbols[d.id] = true
			label := cc.tr(d.item.OptionLabel)
			if label == "" {
				name := d.name
				if d.plur != "" {
					name = d.plur
				}
				label = cc.Catalog.T("add_ingredient", "name", name)
			}
			comp.Options = append(comp.Options, OptionC{ID: d.id, Type: "toggle", Label: label, Default: d.item.Default, L: d.loc})
		}
	}
	// Option `when` conditions are compiled once vars are declared (see vars).
}

type varDraft struct {
	id  string
	src Src
	loc Loc
}

func (cc *cctx) declareVars(f *File) []varDraft {
	var out []varDraft
	for _, kv := range orderedPairs(&f.Vars) {
		id := kv[0].Value
		l := cc.loc(kv[0].Line, kv[0].Column)
		if cc.symbols[id] {
			cc.errAt(l, "var %q clashes with a built-in or option of the same name", id)
			continue
		}
		var s Src
		if err := s.UnmarshalYAML(kv[1]); err != nil {
			cc.errAt(l, "var %q must be an expression", id)
			continue
		}
		cc.symbols[id] = true
		out = append(out, varDraft{id: id, src: s, loc: l})
	}
	return out
}

func (cc *cctx) vars(vars []varDraft, comp *Compiled) {
	byID := map[string]*VarC{}
	deps := map[string][]string{}
	var order []string
	for _, v := range vars {
		e := cc.expr(v.src, "var "+v.id)
		if e == nil {
			continue
		}
		vc := &VarC{ID: v.id, E: e, L: v.loc}
		byID[v.id] = vc
		order = append(order, v.id)
	}
	for id, vc := range byID {
		for _, dep := range vc.E.Vars() {
			if _, ok := byID[dep]; ok {
				deps[id] = append(deps[id], dep)
			}
		}
	}
	// Topological sort keeping declaration order where possible.
	state := map[string]int{} // 0 new, 1 visiting, 2 done
	var visit func(id string, path []string) bool
	visit = func(id string, path []string) bool {
		switch state[id] {
		case 1:
			cc.errAt(byID[id].L, "vars form a cycle: %s", strings.Join(append(path, id), " → "))
			return false
		case 2:
			return true
		}
		state[id] = 1
		for _, d := range deps[id] {
			if !visit(d, append(path, id)) {
				return false
			}
		}
		state[id] = 2
		comp.Vars = append(comp.Vars, *byID[id])
		return true
	}
	for _, id := range order {
		if !visit(id, nil) {
			return
		}
	}
	// Now that every symbol is known, compile option conditions.
	for i := range comp.Options {
		o := &comp.Options[i]
		for _, src := range optionWhens(o.ID, cc) {
			o.When = cc.when(src)
		}
	}
}

// optionWhens finds the raw `when` of an explicit option (kept on the cctx).
func optionWhens(id string, cc *cctx) []Src {
	if s, ok := cc.optWhen[id]; ok {
		return []Src{s}
	}
	return nil
}

func (cc *cctx) times(f *File, comp *Compiled) {
	var total *TimeC
	var parts []*expr.Node
	for _, kv := range orderedPairs(&f.Times) {
		id := kv[0].Value
		var s Src
		if err := s.UnmarshalYAML(kv[1]); err != nil {
			cc.errAt(cc.loc(kv[0].Line, kv[0].Column), "times.%s must be a number or expression", id)
			continue
		}
		e := cc.expr(s, "times."+id)
		if e == nil {
			continue
		}
		label := cc.Catalog.TimeLabel(id)
		tc := TimeC{ID: id, Label: label, E: e, L: cc.loc(kv[0].Line, kv[0].Column)}
		if id == "total" {
			total = &tc
			continue
		}
		parts = append(parts, e.Root)
		comp.Times = append(comp.Times, tc)
	}
	if total == nil && len(parts) > 1 {
		sum := parts[0]
		for _, p := range parts[1:] {
			sum = &expr.Node{Op: "+", Args: []*expr.Node{sum, p}, Pos: -1}
		}
		total = &TimeC{ID: "total", Label: cc.Catalog.TimeLabel("total"), E: &expr.Expr{Root: sum}}
	}
	if total != nil {
		comp.Times = append(comp.Times, *total)
	}
}

func (cc *cctx) equipmentIDs(f *File) {
	seen := map[string]int{}
	for i := range f.Equipment {
		e := &f.Equipment[i]
		if e.ID == "" {
			e.ID = identFrom(e.Name.Text)
			if strings.ContainsAny(e.Name.Text, "{}@&~") || e.ID == "" {
				e.ID = fmt.Sprintf("equipment_%d", i+1)
			}
		}
		if prev, ok := seen[e.ID]; ok && (!e.When.Set || !f.Equipment[prev].When.Set) {
			cc.errAt(cc.loc(e.Line, 0), "duplicate equipment id %q; give each variant a `when:` condition", e.ID)
		}
		seen[e.ID] = i
		cc.eqIDs[e.ID] = true
	}
}

func (cc *cctx) equipment(f *File, comp *Compiled) {
	for _, e := range f.Equipment {
		comp.Equipment = append(comp.Equipment, EquipC{
			ID: e.ID, When: cc.when(e.When), Name: cc.template(e.Name), Note: cc.template(e.Note), L: cc.loc(e.Line, 0),
		})
	}
}

func (cc *cctx) ingredients(comp *Compiled, sections []*sectionDraft, r *Recipe) {
	for _, sd := range sections {
		sc := IngSectionC{Items: []IngC{}}
		if sd.sec != nil {
			sc.Title = cc.tr(sd.sec.Section)
			sc.When = cc.when(sd.sec.When)
			sc.L = cc.loc(sd.sec.Line, 0)
		}
		for _, d := range sd.items {
			it := d.item
			ic := IngC{ID: d.id, Ref: d.ref, Name: d.name, Plural: d.plur, Unit: d.unit, Fixed: it.FixedUnit, Optional: it.Optional, L: d.loc}
			if it.hasOverrides() {
				base := &units.Ing{Name: d.name, Plural: d.plur}
				if d.ref != "" {
					base = cc.db.Entries[d.ref].Info
				}
				info, ds := ApplyIngredientFields(base, DBYAML{
					Measure: it.Measure, Density: it.Density, GramsEach: it.GramsEach,
					UnitGrams: it.UnitGrams, Round: it.Round, Category: it.Category,
				}, cc.Units, d.loc, d.id)
				cc.diags = append(cc.diags, ds...)
				key := fmt.Sprintf("@%s:%d", d.id, len(cc.overrides))
				cc.overrides[key] = info
				ic.Base, ic.Ref = d.ref, key
				if it.Category != "" {
					r.IngCategories[d.id] = it.Category
				}
			}
			if d.ref != "" {
				if cat := cc.db.Entries[d.ref].Category; cat != "" {
					r.IngCategories[d.id] = cat
				}
			}
			ic.When = cc.when(it.When)
			if it.Optional {
				opt := &expr.Node{Op: expr.OpVar, Name: d.id, Pos: -1}
				if ic.When != nil {
					ic.When = &expr.Expr{Root: &expr.Node{Op: "&&", Args: []*expr.Node{opt, ic.When.Root}, Pos: -1}}
				} else {
					ic.When = &expr.Expr{Root: opt}
				}
			}
			if it.Recipe != "" {
				ref := &RecipeRef{Slug: it.Recipe}
				for _, kv := range orderedPairs(&it.With) {
					var src Src
					if err := src.UnmarshalYAML(kv[1]); err != nil {
						cc.errAt(cc.loc(kv[0].Line, kv[0].Column), "with.%s must be an expression", kv[0].Value)
						continue
					}
					if e := cc.expr(src, "with."+kv[0].Value); e != nil {
						ref.With = append(ref.With, WithC{ID: kv[0].Value, E: e})
					}
				}
				ic.Recipe = ref
				if !it.Amount.Set {
					one := 1.0
					ic.Amount = &AmountC{N: &one, S: it.Scale == nil || *it.Scale}
				}
			}
			switch {
			case d.amt != nil:
				ic.Amount = d.amt
			case it.Amount.Set:
				ic.Amount = cc.amountC(it.Amount.Src, it.Scale)
				if ic.Amount != nil && it.Amount.Hi != nil {
					ic.Amount.Hi = cc.amountC(*it.Amount.Hi, it.Scale)
					if it.Recipe != "" {
						cc.errAt(d.loc, "a recipe reference can't use a range amount")
					}
				}
			}
			if ic.Amount != nil && ic.Amount.Hi != nil && it.Split.Kind != 0 {
				cc.errAt(d.loc, "a split ingredient can't also use a range amount")
			}
			if it.Split.Kind != 0 {
				if it.Amount.Set {
					cc.errAt(d.loc, "use either `amount:` or `split:`, not both (the total of a split is its sum)")
				}
				ic.Amount = nil
				for _, kv := range orderedPairs(&it.Split) {
					var src Src
					if err := src.UnmarshalYAML(kv[1]); err != nil {
						cc.errAt(cc.loc(kv[0].Line, kv[0].Column), "split.%s must be a number or expression", kv[0].Value)
						continue
					}
					sc := SplitC{ID: kv[0].Value}
					if src.Num != nil {
						n := *src.Num
						sc.A = AmountC{N: &n, S: it.Scale == nil || *it.Scale}
					} else if e := cc.expr(src, "split."+kv[0].Value); e != nil {
						sc.A = AmountC{E: e}
					}
					ic.Split = append(ic.Split, sc)
				}
			}
			if ic.Amount != nil && ic.Amount.N != nil && *ic.Amount.N < 0 {
				cc.errAt(d.loc, "amount must not be negative")
			}
			ic.Note = cc.template(it.Note)
			sc.Items = append(sc.Items, ic)
		}
		comp.Ingredients = append(comp.Ingredients, sc)
	}
}

// amountC compiles a number (scaled unless scale: false) or an expression.
func (cc *cctx) amountC(s Src, scale *bool) *AmountC {
	if s.Num != nil {
		n := *s.Num
		if n < 0 {
			cc.errAt(s.at(cc.file, 0), "amount must not be negative")
		}
		return &AmountC{N: &n, S: scale == nil || *scale}
	}
	if e := cc.expr(s, "amount"); e != nil {
		return &AmountC{E: e}
	}
	return nil
}

func (cc *cctx) steps(f *File, comp *Compiled) {
	var implicit *StepSectionC
	for _, e := range f.Steps {
		if e.Section != nil {
			if implicit != nil {
				comp.Steps = append(comp.Steps, *implicit)
				implicit = nil
			}
			sc := StepSectionC{Title: cc.tr(e.Section.Section), When: cc.when(e.Section.When), L: cc.loc(e.Section.Line, 0), Items: []StepC{}}
			for _, sub := range e.Section.Steps {
				if sub.Section != nil {
					cc.errAt(cc.loc(sub.Section.Line, 0), "step sections cannot be nested")
					continue
				}
				sc.Items = append(sc.Items, cc.step(sub.Step))
			}
			comp.Steps = append(comp.Steps, sc)
			continue
		}
		if implicit == nil {
			implicit = &StepSectionC{Items: []StepC{}}
		}
		implicit.Items = append(implicit.Items, cc.step(e.Step))
	}
	if implicit != nil {
		comp.Steps = append(comp.Steps, *implicit)
	}
}

func (cc *cctx) step(s *Step) StepC {
	return StepC{When: cc.when(s.When), Text: cc.template(s.Text), Note: cc.template(s.Note), Image: s.Image, L: cc.loc(s.Line, 0)}
}

func (cc *cctx) known(name string) bool { return cc.symbols[name] }

func (cc *cctx) suggestSymbol(name string) string {
	names := make([]string, 0, len(cc.symbols))
	for k := range cc.symbols {
		names = append(names, k)
	}
	sort.Strings(names)
	return suggest(name, names)
}

// expr parses and checks an expression scalar.
func (cc *cctx) expr(s Src, what string) *expr.Expr {
	if !s.Set {
		return nil
	}
	e, err := expr.Parse(s.Text)
	if err == nil {
		err = e.Check(cc.known, cc.suggestSymbol)
	}
	if err != nil {
		pos := 0
		msg := err.Error()
		if pe, ok := err.(*expr.Error); ok {
			pos, msg = pe.Pos, pe.Msg
		}
		cc.errAt(s.at(cc.file, pos), "%s: %s", what, msg)
		return nil
	}
	return e
}

func (cc *cctx) when(s Src) *expr.Expr {
	if !s.Set {
		return nil
	}
	if s.Num != nil {
		cc.errAt(s.at(cc.file, 0), "when: expected a condition, got the number %s", s.Text)
		return nil
	}
	return cc.expr(s, "when")
}

// template parses a text template and checks its expressions and references.
func (cc *cctx) template(s Src) tmpl.Template {
	if !s.Set || s.Text == "" {
		return nil
	}
	if s.Comment != "" && s.Style == 0 {
		cc.diags = append(cc.diags, Diag{File: cc.file, Line: s.Line, Col: s.Col, Warn: true,
			Msg: fmt.Sprintf("YAML treated %q as a comment and dropped it from the text; put the text in quotes to keep it", s.Comment)})
	}
	s.Text = cc.tr(strings.TrimSpace(s.Text))
	t, err := tmpl.Parse(s.Text)
	if err != nil {
		pos := 0
		msg := err.Error()
		if te, ok := err.(*tmpl.Error); ok {
			pos, msg = te.Pos, te.Msg
		}
		cc.errAt(s.at(cc.file, pos), "%s", msg)
		return nil
	}
	_ = t.Walk(func(seg *tmpl.Segment) error {
		at := s.at(cc.file, seg.Pos)
		check := func(e *expr.Expr) {
			if e == nil {
				return
			}
			if err := e.Check(cc.known, cc.suggestSymbol); err != nil {
				msg := err.Error()
				if pe, ok := err.(*expr.Error); ok {
					msg = pe.Msg
				}
				cc.errAt(at, "%s", msg)
			}
		}
		switch seg.Kind {
		case tmpl.KIng:
			if seg.Part != "" && cc.ingIDs[seg.ID] && !cc.parts[seg.ID][seg.Part] {
				msg := fmt.Sprintf("@%s.%s: ingredient %q has no split named %q", seg.ID, seg.Part, seg.ID, seg.Part)
				if sg := suggest(seg.Part, keys(cc.parts[seg.ID])); sg != "" {
					msg += fmt.Sprintf(" (did you mean @%s.%s?)", seg.ID, sg)
				}
				cc.errAt(at, "%s", msg)
			}
			if !cc.ingIDs[seg.ID] {
				msg := fmt.Sprintf("@%s does not match any ingredient id", seg.ID)
				if sg := suggest(seg.ID, keys(cc.ingIDs)); sg != "" {
					msg += fmt.Sprintf(" (did you mean @%s?)", sg)
				}
				cc.errAt(at, "%s; write \\@ for a literal @", msg)
			}
		case tmpl.KEquip:
			if !cc.eqIDs[seg.ID] {
				msg := fmt.Sprintf("&%s does not match any equipment id", seg.ID)
				if sg := suggest(seg.ID, keys(cc.eqIDs)); sg != "" {
					msg += fmt.Sprintf(" (did you mean &%s?)", sg)
				}
				cc.errAt(at, "%s; write \\& for a literal &", msg)
			}
		case tmpl.KExpr:
			if seg.Fmt == "qty" {
				if _, ok := cc.Units.Lookup(seg.Args[0]); !ok {
					cc.errAt(at, "qty: unknown unit %q", seg.Args[0])
				} else {
					x, _ := cc.Units.Lookup(seg.Args[0])
					seg.Args[0] = x.ID
				}
			}
		}
		check(seg.Expr)
		for _, b := range seg.Branches {
			check(b.Cond)
		}
		return nil
	})
	return t
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (it *IngItem) hasOverrides() bool {
	return it.Measure != "" || it.Density != nil || it.GramsEach > 0 || len(it.UnitGrams) > 0 || it.Round != nil || it.Category != ""
}
