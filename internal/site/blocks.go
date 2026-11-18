package site

import (
	"fmt"
	"html"
	"net/url"
	"sort"
	"strings"

	"github.com/RashKash103/get-cooked/internal/expr"
	"github.com/RashKash103/get-cooked/internal/runtime"
)

// The functions in this file render the dynamic regions of a recipe page.
// web/src/ui/blocks.ts mirrors them exactly so the page can re-render when
// the reader changes servings or options. Keep the markup identical.

func esc(s string) string { return html.EscapeString(s) }

// RecipeURL builds the link to a recipe with optional state.
func RecipeURL(base, slug string, servings float64, options map[string]any) string {
	u := base + "recipes/" + slug + "/"
	var q []string
	if servings > 0 {
		q = append(q, "s="+expr.FormatNumber(servings))
	}
	keys := make([]string, 0, len(options))
	for k := range options {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var v string
		switch x := options[k].(type) {
		case bool:
			v = "0"
			if x {
				v = "1"
			}
		case float64:
			v = expr.FormatNumber(x)
		default:
			v = fmt.Sprint(x)
		}
		q = append(q, "o."+url.QueryEscape(k)+"="+url.QueryEscape(v))
	}
	if len(q) > 0 {
		u += "?" + strings.Join(q, "&")
	}
	return u
}

func linkHref(base string, l *runtime.LinkM) string {
	return RecipeURL(base, l.Slug, l.Servings, l.Options)
}

func renderTimes(m *runtime.Model) string {
	var sb strings.Builder
	for _, t := range m.Times {
		cls := "time"
		if t.ID == "total" {
			cls = "time time-total"
		}
		fmt.Fprintf(&sb, `<div class="%s"><dt>%s</dt><dd>%s</dd></div>`, cls, esc(t.Label), esc(t.Text))
	}
	return sb.String()
}

func renderEquipment(m *runtime.Model) string {
	var sb strings.Builder
	for _, e := range m.Equipment {
		sb.WriteString(`<li class="equip">` + esc(e.Name))
		if e.Note != "" {
			sb.WriteString(` <span class="equip-note">` + esc(e.Note) + `</span>`)
		}
		sb.WriteString(`</li>`)
	}
	return sb.String()
}

func renderIngredients(m *runtime.Model, base string) string {
	var sb strings.Builder
	for _, s := range m.Ingredients {
		sb.WriteString(`<section class="ing-section">`)
		if s.Title != "" {
			sb.WriteString(`<h3 class="ing-title">` + esc(s.Title) + `</h3>`)
		}
		sb.WriteString(`<ul class="ing-list">`)
		for _, it := range s.Items {
			fmt.Fprintf(&sb, `<li class="ing" data-id="%s"><label class="ing-label"><input type="checkbox" class="ing-check" data-id="%s"><span class="ing-text">`, esc(it.ID), esc(it.ID))
			if it.Q != nil {
				sb.WriteString(`<span class="qty">` + esc(it.Q.String()) + `</span> `)
			}
			if it.Link != nil {
				fmt.Fprintf(&sb, `<a class="ing-name ing-link" href="%s">%s</a>`, esc(linkHref(base, it.Link)), esc(it.Name))
			} else {
				sb.WriteString(`<span class="ing-name">` + esc(it.Name) + `</span>`)
			}
			if it.Note != "" {
				sb.WriteString(`<span class="ing-note">, ` + esc(it.Note) + `</span>`)
			}
			sb.WriteString(`</span></label></li>`)
		}
		sb.WriteString(`</ul></section>`)
	}
	return sb.String()
}

func renderSegs(segs []runtime.Seg, base string, label string) string {
	var sb strings.Builder
	for _, s := range segs {
		switch s.T {
		case "ing":
			inner := ""
			if s.Q != "" {
				inner = `<span class="ref-q">` + esc(s.Q) + `</span> `
			}
			inner += esc(s.V)
			if s.Link != nil {
				fmt.Fprintf(&sb, `<a class="ref ref-link" href="%s">%s</a>`, esc(linkHref(base, s.Link)), inner)
			} else {
				fmt.Fprintf(&sb, `<span class="ref" data-ing="%s">%s</span>`, esc(s.ID), inner)
			}
		case "eq":
			sb.WriteString(`<span class="ref-eq">` + esc(s.V) + `</span>`)
		case "timer":
			fmt.Fprintf(&sb, `<button type="button" class="timer-chip" data-min="%s" data-label="%s"><svg class="icon" aria-hidden="true"><use href="#i-timer"></use></svg>%s</button>`,
				expr.FormatNumber(s.Min), esc(label), esc(s.V))
		default:
			sb.WriteString(esc(s.V))
		}
	}
	return sb.String()
}

func renderSteps(m *runtime.Model, base string) string {
	var sb strings.Builder
	for _, s := range m.Steps {
		sb.WriteString(`<section class="step-section">`)
		if s.Title != "" {
			sb.WriteString(`<h3 class="step-title">` + esc(s.Title) + `</h3>`)
		}
		sb.WriteString(`<ol class="step-list">`)
		for _, st := range s.Items {
			label := fmt.Sprintf("Step %d", st.N)
			fmt.Fprintf(&sb, `<li class="step" data-n="%d"><button type="button" class="step-num" aria-label="Mark step %d done">%d</button><div class="step-body"><p class="step-text">%s</p>`,
				st.N, st.N, st.N, renderSegs(st.Segs, base, label))
			if st.Note != "" {
				sb.WriteString(`<p class="step-note">` + esc(st.Note) + `</p>`)
			}
			if st.Image != "" {
				fmt.Fprintf(&sb, `<img class="step-img" src="%s" alt="" loading="lazy">`, esc(st.Image))
			}
			sb.WriteString(`</div></li>`)
		}
		sb.WriteString(`</ol></section>`)
	}
	return sb.String()
}

func renderNotes(m *runtime.Model, warnings bool) string {
	var sb strings.Builder
	icons := map[string]string{"warning": "i-alert", "tip": "i-sparkle", "info": "i-info"}
	for _, n := range m.Notes {
		if (n.Kind == "warning") != warnings {
			continue
		}
		fmt.Fprintf(&sb, `<aside class="note note-%s"><svg class="icon" aria-hidden="true"><use href="#%s"></use></svg><p>%s</p></aside>`,
			esc(n.Kind), icons[n.Kind], esc(n.Text))
	}
	return sb.String()
}
