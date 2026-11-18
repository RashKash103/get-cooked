package expr

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

var testEnv = map[string]Value{
	"servings":    Num(4),
	"scale":       Num(0.5),
	"cranberries": Bool(true),
	"sweetness":   Str("less"),
	"zero":        Num(0),
}

type vector struct {
	Src   string `json:"src"`
	IR    *Expr  `json:"ir,omitempty"`
	Want  any    `json:"want,omitempty"`
	Error string `json:"error,omitempty"`
}

var vectors = []vector{
	{Src: "1 + 2 * 3", Want: 7.0},
	{Src: "(1 + 2) * 3", Want: 9.0},
	{Src: "2 ^ 3 ^ 2", Want: 512.0},
	{Src: "-2 ^ 2", Want: -4.0},
	{Src: "2 ^ -1", Want: 0.5},
	{Src: "30 + servings * 2", Want: 38.0},
	{Src: "servings <= 2 ? '8x8' : '13x9'", Want: "13x9"},
	{Src: "servings > 6 ? 'a' : servings > 3 ? 'b' : 'c'", Want: "b"},
	{Src: "cranberries && servings > 1", Want: true},
	{Src: "!cranberries || false", Want: false},
	{Src: "sweetness == 'less' ? 0.7 : 1", Want: 0.7},
	{Src: "sweetness != \"less\"", Want: false},
	{Src: "10 % 3", Want: 1.0},
	{Src: "7 / 2", Want: 3.5},
	{Src: "min(3, 1, 2) + max(1, 5)", Want: 6.0},
	{Src: "round(2.5) + round(-2.5)", Want: 0.0},
	{Src: "round(2.4)", Want: 2.0},
	{Src: "round(7, 5)", Want: 5.0},
	{Src: "round(7.5, 5)", Want: 10.0},
	{Src: "floor(7.9) + ceil(1.1)", Want: 9.0},
	{Src: "ceil(11, 6)", Want: 12.0},
	{Src: "clamp(servings, 1, 3)", Want: 3.0},
	{Src: "abs(-3)", Want: 3.0},
	{Src: "sqrt(16)", Want: 4.0},
	{Src: "pow(2, 10)", Want: 1024.0},
	{Src: "lerp(20, 30, scale)", Want: 25.0},
	{Src: "if(servings == 4, 'four', 'other')", Want: "four"},
	{Src: "plural(servings, 'cookie')", Want: "cookies"},
	{Src: "plural(1, 'loaf', 'loaves')", Want: "loaf"},
	{Src: "plural(2, 'loaf', 'loaves')", Want: "loaves"},
	{Src: "'x' + 1.256", Want: "x1.26"},
	{Src: "str(1/3)", Want: "0.33"},
	{Src: "1_000 + .5", Want: 1000.5},
	{Src: "3 × 2", Want: 6.0},
	{Src: "false ? 1/zero : 2", Want: 2.0},
	{Src: "1 / zero", Error: "division by zero"},
	{Src: "1 % zero", Error: "modulo by zero"},
	{Src: "1 + true", Error: "cannot add number and boolean"},
	{Src: "servings == 'four'", Error: "cannot compare"},
	{Src: "servings ? 1 : 2", Error: "expects true/false"},
	{Src: "'a' < 'b'", Error: "expects a number"},
	{Src: "!servings", Error: "expects true/false"},
	{Src: "sqrt(-1)", Error: "negative"},
	{Src: "round(1, 0)", Error: "step must be positive"},
}

func TestVectors(t *testing.T) {
	env := MapEnv(testEnv)
	for i, v := range vectors {
		e, err := Parse(v.Src)
		if err != nil {
			t.Fatalf("%s: parse: %v", v.Src, err)
		}
		vectors[i].IR = e
		got, err := e.Eval(env)
		if v.Error != "" {
			if err == nil || !strings.Contains(err.Error(), v.Error) {
				t.Errorf("%s: want error containing %q, got %v (%v)", v.Src, v.Error, err, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", v.Src, err)
			continue
		}
		if got.Interface() != v.Want {
			t.Errorf("%s = %#v, want %#v", v.Src, got.Interface(), v.Want)
		}
		// IR round trip must evaluate identically.
		b, _ := json.Marshal(e)
		var back Expr
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		got2, err := back.Eval(env)
		if err != nil || got2 != got {
			t.Errorf("%s: IR round trip gave %v, %v", v.Src, got2, err)
		}
	}
	envJSON := map[string]any{}
	for k, v := range testEnv {
		envJSON[k] = v.Interface()
	}
	golden(t, filepath.Join("..", "..", "testdata", "conformance", "expr-vectors.json"),
		map[string]any{"env": envJSON, "vectors": vectors})
}

func golden(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run go test ./... -update): %v", path, err)
	}
	if string(want) != string(b) {
		t.Errorf("golden %s is out of date; run: go test ./internal/expr -update", path)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"", "empty expression"},
		{"1 +", "unexpected end"},
		{"(1 + 2", `expected ")"`},
		{"1 2", `unexpected "2"`},
		{"'abc", "unterminated string"},
		{"a ? b", `expected ":"`},
		{"1 $ 2", "unexpected character"},
		{"1..2", "invalid number"},
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Parse(%q) error = %v, want %q", c.src, err, c.want)
		}
	}
}

func TestCheck(t *testing.T) {
	known := func(s string) bool { return s == "servings" || s == "scale" }
	suggest := func(s string) string {
		if s == "serving" {
			return "servings"
		}
		return ""
	}
	cases := []struct{ src, want string }{
		{"servings * scale", ""},
		{"serving * 2", `unknown variable "serving" (did you mean "servings"?)`},
		{"foo(1)", `unknown function "foo"`},
		{"min(1)", "min() takes 2 or more arguments, got 1"},
		{"round(1, 2, 3)", "round() takes 1 to 2 arguments, got 3"},
		{"abs()", "abs() takes 1 arguments"},
	}
	for _, c := range cases {
		e, err := Parse(c.src)
		if err != nil {
			t.Fatal(err)
		}
		err = e.Check(known, suggest)
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: unexpected %v", c.src, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.src, err, c.want)
		}
	}
}

func TestErrorPosition(t *testing.T) {
	_, err := Parse("servings + * 2")
	e, ok := err.(*Error)
	if !ok || e.Pos != 11 {
		t.Fatalf("got %#v", err)
	}
}

func TestVarsAndFormat(t *testing.T) {
	e, _ := Parse("a + b * a + min(c, 1)")
	if got := strings.Join(e.Vars(), ","); got != "a,b,c" {
		t.Errorf("Vars = %s", got)
	}
	for in, want := range map[float64]string{1: "1", 0.333333: "0.33", 2.005: "2.01", -0.001: "0", 1e6: "1000000"} {
		if got := FormatNumber(in); got != want {
			t.Errorf("FormatNumber(%v) = %s want %s", in, got, want)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, v := range vectors {
		f.Add(v.Src)
	}
	f.Fuzz(func(t *testing.T, src string) {
		e, err := Parse(src)
		if err != nil {
			return
		}
		_, _ = e.Eval(MapEnv(testEnv))
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		var back Expr
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("round trip %s: %v", b, err)
		}
	})
}
