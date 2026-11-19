// Package export renders a recipe (for one state) as Markdown, plain text or
// Typst (and PDF through the typst CLI). The Markdown output is mirrored by
// web/src/ui/export.ts so browser downloads match `get-cooked export`.
package export

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/RashKash103/get-cooked/internal/expr"
	"github.com/RashKash103/get-cooked/internal/i18n"
	"github.com/RashKash103/get-cooked/internal/recipe"
	"github.com/RashKash103/get-cooked/internal/runtime"
)

// Doc is everything an exporter needs.
type Doc struct {
	R    *recipe.Recipe
	M    *runtime.Model
	Cat  *i18n.Catalog
	Opts map[string]any // the state's option values (nil = defaults)
}

// Summary describes the chosen options ("Sweeten with: Maple syrup · Add
// dried cranberries · Units: Metric").
func Summary(d Doc) string {
	var parts []string
	for _, o := range d.R.C.Options {
		if !contains(d.M.Options, o.ID) {
			continue
		}
		v, ok := d.Opts[o.ID]
		if !ok {
			v = o.Default
		}
		switch o.Type {
		case "toggle":
			if b, _ := v.(bool); b {
				parts = append(parts, o.Label)
			}
		case "choice":
			if o.Swap != "" && v == o.Default {
				continue // nothing swapped
			}
			for _, c := range o.Choices {
				if c.ID == v {
					parts = append(parts, o.Label+": "+c.Label)
				}
			}
		default:
			f, _ := v.(float64)
			parts = append(parts, o.Label+": "+d.Cat.Num(expr.FormatNumber(f))+o.Unit)
		}
	}
	if d.M.System != "original" {
		parts = append(parts, d.Cat.T("units")+": "+d.Cat.T("units_"+d.M.System))
	}
	return strings.Join(parts, " · ")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// StepText flattens a step's segments.
func StepText(segs []runtime.Seg) string {
	var sb strings.Builder
	for _, s := range segs {
		if s.T == "ing" && s.Q != "" {
			sb.WriteString(s.Q + s.J)
		}
		sb.WriteString(s.V)
	}
	return sb.String()
}

// IngLine formats one ingredient ("2 cups rolled oats, packed").
func IngLine(it runtime.IngM) string {
	s := it.Name
	if it.Q != nil {
		s = it.Q.String() + it.J + s
	}
	if it.Note != "" {
		s += ", " + it.Note
	}
	return s
}

func uses(d Doc, it runtime.IngM) string {
	if len(it.Uses) == 0 {
		return ""
	}
	parts := make([]string, len(it.Uses))
	for i, u := range it.Uses {
		parts[i] = d.Cat.T("uses", "q", u.Q, "n", d.Cat.Num(fmt.Sprint(u.N)))
	}
	return strings.Join(parts, " · ")
}

func metaLine(d Doc) string {
	parts := []string{d.M.Yield}
	for _, t := range d.M.Times {
		parts = append(parts, t.Label+" "+t.Text)
	}
	return strings.Join(parts, " · ")
}

// Markdown renders the recipe as Markdown.
func Markdown(d Doc) string {
	var b strings.Builder
	m := d.M
	fmt.Fprintf(&b, "# %s\n\n", d.R.Title)
	if d.R.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", d.R.Description)
	}
	fmt.Fprintf(&b, "**%s**\n\n", metaLine(d))
	if s := Summary(d); s != "" {
		fmt.Fprintf(&b, "*%s*\n\n", s)
	}
	for _, n := range m.Notes {
		if n.Kind == "warning" {
			fmt.Fprintf(&b, "> ⚠ %s\n\n", n.Text)
		}
	}
	fmt.Fprintf(&b, "## %s\n\n", d.Cat.T("ingredients"))
	for _, s := range m.Ingredients {
		if s.Title != "" {
			fmt.Fprintf(&b, "### %s\n\n", s.Title)
		}
		for _, it := range s.Items {
			line := IngLine(it)
			if u := uses(d, it); u != "" {
				line += " (" + u + ")"
			}
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	if len(m.Equipment) > 0 {
		fmt.Fprintf(&b, "## %s\n\n", d.Cat.T("equipment"))
		for _, e := range m.Equipment {
			line := e.Name
			if e.Note != "" {
				line += ", " + e.Note
			}
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "## %s\n\n", d.Cat.T("steps"))
	for _, s := range m.Steps {
		if s.Title != "" {
			fmt.Fprintf(&b, "### %s\n\n", s.Title)
		}
		for _, st := range s.Items {
			fmt.Fprintf(&b, "%s. %s\n", d.Cat.Num(fmt.Sprint(st.N)), StepText(st.Segs))
			if st.Note != "" {
				fmt.Fprintf(&b, "   *%s*\n", st.Note)
			}
		}
		b.WriteString("\n")
	}
	var tips []string
	for _, n := range m.Notes {
		if n.Kind != "warning" {
			tips = append(tips, n.Text)
		}
	}
	if len(tips) > 0 {
		fmt.Fprintf(&b, "## %s\n\n", d.Cat.T("notes"))
		for _, t := range tips {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		b.WriteString("\n")
	}
	if src := d.R.Source; src != nil && (src.Name != "" || src.URL != "") {
		name := src.Name
		if name == "" {
			name = src.URL
		}
		if src.URL != "" {
			fmt.Fprintf(&b, "%s: [%s](%s)\n", d.Cat.T("source"), name, src.URL)
		} else {
			fmt.Fprintf(&b, "%s: %s\n", d.Cat.T("source"), name)
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// Text renders plain text (Markdown without the markup).
func Text(d Doc) string {
	r := strings.NewReplacer("**", "", "*", "", "> ", "", "### ", "", "## ", "", "# ", "")
	var out []string
	for _, line := range strings.Split(Markdown(d), "\n") {
		out = append(out, r.Replace(line))
	}
	return strings.Join(out, "\n")
}

// ts quotes text as a Typst string for code mode: "…".
func ts(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ").Replace(s) + `"`
}

// nb keeps a quantity on one line ("5 ml", "1 ½ cups").
func nb(s string) string { return strings.ReplaceAll(s, " ", "\u00a0") }

var fracRe = regexp.MustCompile(`(\p{Nd}+)\x{200E}?\x{2044}(\p{Nd}+)`)

// typFrac is typ, but sets fraction-slash fractions ("١⁄٢") with a small
// raised numerator and a small lowered denominator.
func typFrac(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range fracRe.FindAllStringSubmatchIndex(s, -1) {
		if m[0] > last {
			b.WriteString(typ(s[last:m[0]]))
		}
		fmt.Fprintf(&b, "#box[#text(size: 0.62em, baseline: -0.42em)[%s]\u2044#text(size: 0.62em, baseline: 0.08em)[%s]]", typ(s[m[2]:m[3]]), typ(s[m[4]:m[5]]))
		last = m[1]
	}
	if last < len(s) {
		b.WriteString(typ(s[last:]))
	}
	return b.String()
}

// typ inserts text in markup mode: #"…" (nothing in it is interpreted).
func typ(s string) string { return "#" + ts(s) }

// Paper sizes accepted by Typst.
var Paper = map[string]bool{"a4": true, "us-letter": true, "a5": true}

// Typst renders a Typst document laid out like a good cookbook page:
// ingredients in a narrow column beside numbered steps.
func Typst(d Doc, paper string) string {
	if !Paper[paper] {
		paper = "a4"
	}
	m := d.M
	mk := typ
	if d.Cat.Diagonal() {
		mk = typFrac
	}
	var b strings.Builder
	accent := "#c2410c"
	fmt.Fprintf(&b, `#set document(title: %s)
#set page(paper: %q, margin: (x: 17mm, top: 16mm, bottom: 18mm),
  footer: context [#set text(8pt, fill: luma(130)); %s #h(1fr) #counter(page).display()])
#set text(font: ("Libertinus Serif", "New Computer Modern"), size: 11pt, lang: %q)
#set par(leading: 0.62em, justify: false)
#set list(marker: none, body-indent: 0pt, spacing: 0.85em)
#let digits = %s
#let localnum(n) = if digits.len() == 10 { str(n).replace(regex("[0-9]"), m => digits.at(int(m.text))) } else { str(n) }
#set enum(spacing: 1.05em, body-indent: 0.7em, numbering: n => text(weight: "bold", fill: rgb(%q))[#localnum(n)])
#let section(t) = block(above: 1.3em, below: 0.7em, sticky: true, text(size: 8.5pt, weight: "bold", tracking: 0.09em, fill: rgb(%q), upper(t)))
#let sub(t) = block(above: 1.1em, below: 0.45em, sticky: true, text(size: 9.5pt, style: "italic", fill: luma(90), t))
#show list.item: it => block(breakable: false, spacing: 0.85em, it)
#show enum.item: it => block(breakable: false, spacing: 1.05em, it)
#let qty(t) = text(weight: "bold", t)
#let muted(t) = text(fill: luma(105), t)

`, ts(d.R.Title), paper, typ(d.R.Title), d.Cat.Lang, typstDigits(d.Cat.Digits), accent, accent)
	fmt.Fprintf(&b, "#block(below: 1em, text(size: 23pt, weight: \"bold\", %s))\n", ts(d.R.Title))
	if d.R.Description != "" {
		fmt.Fprintf(&b, "#muted[%s]\n\n", typ(d.R.Description))
	}
	fmt.Fprintf(&b, "#text(size: 9.5pt)[%s]\n\n", mk(metaLine(d)))
	if s := Summary(d); s != "" {
		fmt.Fprintf(&b, "#text(size: 9.5pt, style: \"italic\", fill: luma(90))[%s]\n\n", typ(s))
	}
	for _, n := range m.Notes {
		if n.Kind == "warning" {
			fmt.Fprintf(&b, "#block(fill: rgb(\"#fef3c7\"), inset: 8pt, radius: 4pt, width: 100%%)[#text(size: 10pt)[%s]]\n\n", mk("⚠ "+n.Text))
		}
	}
	// Left column: ingredients and equipment.
	var left strings.Builder
	fmt.Fprintf(&left, "#section(%s)\n", ts(d.Cat.T("ingredients")))
	for _, s := range m.Ingredients {
		if s.Title != "" {
			fmt.Fprintf(&left, "#sub(%s)\n", ts(s.Title))
		}
		for _, it := range s.Items {
			left.WriteString("- ")
			if it.Q != nil {
				fmt.Fprintf(&left, "#qty[%s]%s", mk(nb(it.Q.String())), typ(it.J))
			}
			left.WriteString(typ(it.Name))
			if it.Note != "" {
				fmt.Fprintf(&left, "#muted(%s)", ts(", "+it.Note))
			}
			if u := uses(d, it); u != "" {
				fmt.Fprintf(&left, "\\ #text(size: 8.5pt, fill: luma(110))[%s]", mk(u))
			}
			left.WriteString("\n")
		}
	}
	if len(m.Equipment) > 0 {
		fmt.Fprintf(&left, "#section(%s)\n", ts(d.Cat.T("equipment")))
		for _, e := range m.Equipment {
			left.WriteString("- " + typ(e.Name))
			if e.Note != "" {
				fmt.Fprintf(&left, "#muted(%s)", ts(", "+e.Note))
			}
			left.WriteString("\n")
		}
	}
	// Right column: steps.
	var right strings.Builder
	fmt.Fprintf(&right, "#section(%s)\n", ts(d.Cat.T("steps")))
	for _, s := range m.Steps {
		if s.Title != "" {
			fmt.Fprintf(&right, "#sub(%s)\n", ts(s.Title))
		}
		for _, st := range s.Items {
			fmt.Fprintf(&right, "%d. ", st.N)
			for _, seg := range st.Segs {
				if seg.T == "ing" && seg.Q != "" {
					fmt.Fprintf(&right, "#qty[%s]%s", mk(nb(seg.Q)), typ(seg.J))
				}
				right.WriteString(mk(seg.V))
			}
			if st.Note != "" {
				fmt.Fprintf(&right, "\\ #muted(text(size: 9.5pt, style: \"italic\", %s))", ts(st.Note))
			}
			right.WriteString("\n")
		}
	}
	b.WriteString("#v(2pt)\n#line(length: 100%, stroke: 0.5pt + luma(205))\n")
	fmt.Fprintf(&b, "#grid(columns: (36%%, 1fr), column-gutter: 9mm,\n[\n%s],\n[\n%s])\n\n", left.String(), right.String())
	var tips []string
	for _, n := range m.Notes {
		if n.Kind != "warning" {
			tips = append(tips, n.Text)
		}
	}
	if len(tips) > 0 {
		fmt.Fprintf(&b, "#section(%s)\n", ts(d.Cat.T("notes")))
		for _, t := range tips {
			fmt.Fprintf(&b, "- %s\n", mk("• "+t))
		}
	}
	if src := d.R.Source; src != nil && (src.Name != "" || src.URL != "") {
		name := src.Name
		if name == "" {
			name = src.URL
		}
		fmt.Fprintf(&b, "\n#v(1em)\n#text(size: 8.5pt, fill: luma(110))[%s]\n", typ(d.Cat.T("source")+": "+name+" "+src.URL))
	}
	return b.String()
}

// typstDigits renders the digit list as a Typst array ("()" for none).
func typstDigits(digits string) string {
	if digits == "" {
		return "()"
	}
	parts := make([]string, 0, 10)
	for _, r := range digits {
		parts = append(parts, ts(string(r)))
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// ErrNoTypst is returned when the typst CLI is not installed.
var ErrNoTypst = errors.New("PDF export needs the typst CLI (https://typst.app); install it, e.g. `brew install typst`, `cargo install typst-cli` or download it from https://github.com/typst/typst/releases")

// PDF compiles Typst source with the typst CLI.
func PDF(ctx context.Context, source string) ([]byte, error) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		return nil, ErrNoTypst
	}
	dir, err := os.MkdirTemp("", "get-cooked-typst-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	in, out := filepath.Join(dir, "recipe.typ"), filepath.Join(dir, "recipe.pdf")
	if err := os.WriteFile(in, []byte(source), 0o644); err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "compile", in, out)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("typst: %w\n%s", err, stderr.String())
	}
	return os.ReadFile(out)
}
