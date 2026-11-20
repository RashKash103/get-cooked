package recipe

import (
	"fmt"
	"sort"

	"github.com/RashKash103/get-cooked/internal/units"
)

// Link resolves recipe references (ingredients with `recipe:`) across a set
// of compiled recipes, filling in the target's servings unit and limits and
// checking that passed options exist. An ingredient from the database that
// another recipe makes (`makes: { ingredient: … }`) links to that recipe.
func Link(recipes []*Recipe) Diags {
	bySlug := map[string]*Recipe{}
	slugs := make([]string, 0, len(recipes))
	for _, r := range recipes {
		bySlug[r.Slug] = r
		slugs = append(slugs, r.Slug)
	}
	sort.Strings(slugs)
	var diags Diags
	// Which recipe makes each database ingredient (the first by slug).
	makers := map[string]*Recipe{}
	for _, slug := range slugs {
		r := bySlug[slug]
		if m := r.C.Makes; m != nil && m.Base != "" {
			if other, ok := makers[m.Base]; ok {
				diags = append(diags, Diag{File: r.Path, Warn: true, Msg: fmt.Sprintf("%q is also made by %q; ingredients link to %q", m.Base, other.Slug, other.Slug)})
				continue
			}
			makers[m.Base] = r
		}
	}
	for _, r := range recipes {
		for si := range r.C.Ingredients {
			for ii := range r.C.Ingredients[si].Items {
				ic := &r.C.Ingredients[si].Items[ii]
				ref := ic.Recipe
				if ref == nil {
					key := ic.Base
					if key == "" {
						key = ic.Ref
					}
					if t := makers[key]; t != nil && t != r && key != "" {
						ref := &RecipeRef{Slug: t.Slug}
						if err := measure(r, t, ic, ref); err != nil {
							d := ic.L.Diag("%q is made by recipe %q, but %v; it won't link there", ic.Name, t.Slug, err)
							d.Warn = true
							diags = append(diags, d)
						} else {
							ic.Recipe = ref
						}
					}
					continue
				}
				t, ok := bySlug[ref.Slug]
				if !ok {
					msg := fmt.Sprintf("recipe %q not found", ref.Slug)
					if s := suggest(ref.Slug, slugs); s != "" {
						msg += fmt.Sprintf(" (did you mean %q?)", s)
					}
					diags = append(diags, ic.L.Diag("%s", msg))
					continue
				}
				if t == r {
					diags = append(diags, ic.L.Diag("a recipe cannot reference itself"))
					continue
				}
				s := t.C.Servings
				ref.One, ref.Other, ref.Min, ref.Max, ref.Step = s.One, s.Other, s.Min, s.Max, s.Step
				if ic.Unit != "" {
					// Measured by what the target makes ("2 tsp garam masala").
					if t.C.Makes == nil {
						diags = append(diags, ic.L.Diag("recipe %q doesn't say what it makes, so it can't be measured in %s; add `makes:` to it (e.g. makes: { amount: 0.5, unit: cup }) or remove `unit:`", t.Slug, ic.Unit))
						continue
					}
					if err := measure(r, t, ic, ref); err != nil {
						diags = append(diags, ic.L.Diag("%v", err))
						continue
					}
					m := t.C.Makes
					ic.Ref, ic.Base = m.Key, m.Base
					if ic.Name == "" && m.Name != "" {
						ic.Name, ic.Plural = m.Name, m.Plural
					}
				}
				if ic.Name == "" {
					ic.Name = t.Title
				}
				for wi := range ref.With {
					w := &ref.With[wi]
					found := false
					for _, o := range t.C.Options {
						if o.ID == w.ID {
							w.Type, found = o.Type, true
						}
					}
					if !found {
						var ids []string
						for _, o := range t.C.Options {
							ids = append(ids, o.ID)
						}
						msg := fmt.Sprintf("with.%s: recipe %q has no option %q", w.ID, t.Slug, w.ID)
						if s := suggest(w.ID, ids); s != "" {
							msg += fmt.Sprintf(" (did you mean %q?)", s)
						}
						diags = append(diags, ic.L.Diag("%s", msg))
					}
				}
			}
		}
	}
	return diags
}

// measure sets up ref for an ingredient measured by what t makes: the unit
// and amount per serving, with t's conversion info copied into r.
func measure(r, t *Recipe, ic *IngC, ref *RecipeRef) error {
	m := t.C.Makes
	info := t.C.Ings[m.Key]
	unit := ic.Unit
	if unit == "" {
		unit = "each"
	}
	if _, ok := r.C.Units.Convert(1, unit, m.Unit, info); !ok && ic.Amount != nil {
		return fmt.Errorf("%s can't be converted to %s (what %q makes is measured in %s); add a density or weight to its `makes:`", unitName(r.C.Units, unit), unitName(r.C.Units, m.Unit), t.Slug, m.Unit)
	}
	s := t.C.Servings
	ref.One, ref.Other, ref.Min, ref.Max, ref.Step = s.One, s.Other, s.Min, s.Max, s.Step
	ref.Unit, ref.Per, ref.Key = m.Unit, m.Amount/s.Default, m.Key
	r.C.Ings[m.Key] = info
	return nil
}

func unitName(t *units.Table, id string) string {
	if u, ok := t.Units[id]; ok {
		return u.Other
	}
	return id
}
