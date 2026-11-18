package export_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/RashKash103/get-cooked/internal/export"
	"github.com/RashKash103/get-cooked/internal/runtime"
	"github.com/RashKash103/get-cooked/internal/site"
)

func doc(t *testing.T, slug, lang string, st runtime.State) export.Doc {
	t.Helper()
	s, err := site.Load("../../examples/site", site.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range s.Langs {
		if l.Code != lang {
			continue
		}
		for _, r := range l.Recipes {
			if r.Slug == slug {
				m, err := runtime.Render(r.C, st)
				if err != nil {
					t.Fatal(err)
				}
				opts := map[string]any{}
				for _, o := range r.C.Options {
					opts[o.ID] = o.Default
				}
				for k, v := range st.Options {
					opts[k] = v
				}
				return export.Doc{R: r, M: m, Cat: l.Catalog, Opts: opts}
			}
		}
	}
	t.Fatalf("no %s/%s", lang, slug)
	return export.Doc{}
}

func TestMarkdown(t *testing.T) {
	d := doc(t, "cranberry-baked-oatmeal", "en", runtime.State{Servings: 1, System: "metric", Options: map[string]any{"walnuts": true}})
	md := export.Markdown(d)
	for _, want := range []string{
		"# Cranberry Baked Oatmeal\n",
		"**1 serving · Prep 10 min · Bake 25 min · Total 35 min**",
		"*Sweeten with: Maple syrup · Add dried cranberries · Add walnuts · Units: Metric*",
		"> ⚠ A single serving",
		"### Oats\n\n- 45 g rolled oats\n",
		"(", "for step 3",
		"1. Heat the oven to 190°C and butter the 8×8 inch baking dish.",
		"## Notes\n\n- Leftovers keep",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	txt := export.Text(d)
	if strings.Contains(txt, "**") || strings.Contains(txt, "## ") || !strings.Contains(txt, "Cranberry Baked Oatmeal") {
		t.Errorf("text:\n%s", txt)
	}
	es := export.Markdown(doc(t, "cranberry-baked-oatmeal", "es", runtime.State{}))
	if !strings.Contains(es, "## Ingredientes") || !strings.Contains(es, "180 g de copos de avena") {
		t.Errorf("es markdown:\n%s", es)
	}
}

func TestTypst(t *testing.T) {
	d := doc(t, "crispy-chicken-thighs", "en", runtime.State{Options: map[string]any{"heat": 3.0}})
	src := export.Typst(d, "us-letter")
	for _, want := range []string{
		`#set page(paper: "us-letter"`,
		`#text(size: 23pt, weight: "bold")[#"Crispy Smoked-Paprika Chicken Thighs"]`,
		`#section("Ingredients")`,
		`#qty("6")`,
		`#grid(columns: (36%, 1fr)`,
		`#"⚠ Spice level 3 is genuinely hot.`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("typst missing %q", want)
		}
	}
	// Quotes and backslashes in text are escaped.
	if strings.Contains(src, `"you'll cook`) && !strings.Contains(src, `you'll`) {
		t.Error("escaping")
	}
	if _, err := exec.LookPath("typst"); err != nil {
		if _, err := export.PDF(context.Background(), src); !errors.Is(err, export.ErrNoTypst) {
			t.Errorf("want ErrNoTypst, got %v", err)
		}
		t.Skip("typst not installed; skipping PDF compilation")
	}
	pdf, err := export.PDF(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(pdf), "%PDF") {
		t.Error("not a PDF")
	}
}
