package tmpl

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(strings.TrimSpace(sb.String()), `\u0026`, "&")
}

func TestParse(t *testing.T) {
	cases := []struct{ src, want string }{
		{"Plain text.", `["Plain text."]`},
		{"Bake {bake} min", `["Bake ",{"e":["v","bake"]}," min"]`},
		{"Heat to {350 | temp}.", `["Heat to ",{"e":["n",350],"f":"temp"},"."]`},
		{"Reserve {0.5 | qty:cup}", `["Reserve ",{"a":["cup"],"e":["n",0.5],"f":"qty"}]`},
		{"{a || b ? 'x' : 'y'}", `[{"e":["?",["||",["v","a"],["v","b"]],["s","x"],["s","y"]]}]`},
		{"Add @flour and @salt.", `["Add ",{"ing":"flour"}," and ",{"ing":"salt"},"."]`},
		{"Add @sugar{1/2}", `["Add ",{"ing":"sugar","p":["/",["n",1],["n",2]]}]`},
		{"the @olive-oil{-}", `["the ",{"ing":"olive-oil","n":true}]`},
		{"line a &pan.", `["line a ",{"eq":"pan"},"."]`},
		{"salt & pepper", `["salt & pepper"]`},
		{"bake ~{bake + 2}", `["bake ",{"timer":["+",["v","bake"],["n",2]]}]`},
		{"email me@ 3 & 2 ~ x #1", `["email me@ 3 & 2 ~ x #1"]`},
		{`literal \{braces\} \@x \&y \~\{z\}`, `["literal {braces} @x &y ~{z}"]`},
		{"oats{if c}, @c{end}.", `["oats",{"if":[{"c":["v","c"],"t":[", ",{"ing":"c"}]}]},"."]`},
		{"{if a}A{elif b}B{else}C{end}", `[{"else":["C"],"if":[{"c":["v","a"],"t":["A"]},{"c":["v","b"],"t":["B"]}]}]`},
		{"{if a}{if b}AB{end}{end}", `[{"if":[{"c":["v","a"],"t":[{"if":[{"c":["v","b"],"t":["AB"]}]}]}]}]`},
		{"{ s == '}' }", `[{"e":["==",["v","s"],["s","}"]]}]`},
		{"Add @salt-", `["Add ",{"ing":"salt"},"-"]`},
	}
	for _, c := range cases {
		got, err := Parse(c.src)
		if err != nil {
			t.Errorf("%q: %v", c.src, err)
			continue
		}
		if s := mustJSON(t, got); s != c.want {
			t.Errorf("%q\n got %s\nwant %s", c.src, s, c.want)
		}
		var back Template
		if err := json.Unmarshal([]byte(mustJSON(t, got)), &back); err != nil {
			t.Fatal(err)
		}
		if s := mustJSON(t, back); s != c.want {
			t.Errorf("%q round trip: %s", c.src, s)
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		src, want string
		pos       int
	}{
		{"oops {", "unclosed", 5},
		{"oops }", "unmatched", 5},
		{"{if a}x", "missing its {end}", 0},
		{"x{end}", "{end} without", 1},
		{"x{else}", "{else} without", 1},
		{"{if a}{else}{elif b}{end}", "{elif} without", 12},
		{"{1 +}", "unexpected end", 4},
		{"{x | nope}", "unknown formatter", 3},
		{"{x | qty}", "takes 1 argument", 3},
		{"{a {b}}", `unexpected "{"`, 3},
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v want %q", c.src, err, c.want)
			continue
		}
		if e, ok := err.(*Error); ok && e.Pos != c.pos {
			t.Errorf("%q: pos %d want %d", c.src, e.Pos, c.pos)
		}
	}
}

func TestWalkAndStatic(t *testing.T) {
	tp, _ := Parse("{if a}@x{else}{b}{end} &y ~{c}")
	var kinds []string
	_ = tp.Walk(func(s *Segment) error { kinds = append(kinds, s.Kind); return nil })
	if got := strings.Join(kinds, ","); got != "if,ing,expr,text,eq,text,timer" {
		t.Errorf("walk = %s", got)
	}
	if len(tp.Exprs()) != 3 {
		t.Errorf("exprs = %d", len(tp.Exprs()))
	}
	if tp.IsStatic() || !Plain("x").IsStatic() || Plain("x").String() != "x" {
		t.Error("static checks")
	}
}
