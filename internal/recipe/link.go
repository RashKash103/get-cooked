package recipe

import (
	"fmt"
	"sort"
)

// Link resolves recipe references (ingredients with `recipe:`) across a set
// of compiled recipes, filling in the target's servings unit and limits and
// checking that passed options exist.
func Link(recipes []*Recipe) Diags {
	bySlug := map[string]*Recipe{}
	slugs := make([]string, 0, len(recipes))
	for _, r := range recipes {
		bySlug[r.Slug] = r
		slugs = append(slugs, r.Slug)
	}
	sort.Strings(slugs)
	var diags Diags
	for _, r := range recipes {
		for si := range r.C.Ingredients {
			for ii := range r.C.Ingredients[si].Items {
				ic := &r.C.Ingredients[si].Items[ii]
				ref := ic.Recipe
				if ref == nil {
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
