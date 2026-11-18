package expr

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Node operators. Binary operators use their source spelling.
const (
	OpNum  = "n"
	OpStr  = "s"
	OpBool = "b"
	OpVar  = "v"
	OpCall = "call"
	OpNeg  = "neg"
	OpNot  = "!"
	OpCond = "?"
)

// Node is an expression AST node.
type Node struct {
	Op   string
	Num  float64
	Str  string
	Bool bool
	Name string
	Args []*Node
	Pos  int
}

// Expr is a parsed expression. It marshals to / from the compact JSON IR.
type Expr struct {
	Root *Node
	Src  string
}

// FuncSpec describes a built-in function's arity.
type FuncSpec struct {
	Min, Max int // Max < 0 means variadic
}

// Funcs lists the built-in functions.
var Funcs = map[string]FuncSpec{
	"min":    {2, -1},
	"max":    {2, -1},
	"abs":    {1, 1},
	"round":  {1, 2},
	"floor":  {1, 2},
	"ceil":   {1, 2},
	"clamp":  {3, 3},
	"sqrt":   {1, 1},
	"pow":    {2, 2},
	"lerp":   {3, 3},
	"if":     {3, 3},
	"plural": {2, 3},
	"str":    {1, 1},
}

// Vars returns the distinct identifiers referenced by the expression.
func (e *Expr) Vars() []string {
	seen := map[string]bool{}
	var walk func(n *Node)
	walk = func(n *Node) {
		if n.Op == OpVar {
			seen[n.Name] = true
		}
		for _, a := range n.Args {
			walk(a)
		}
	}
	walk(e.Root)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Check statically validates identifiers and function calls. known reports
// whether an identifier is defined.
func (e *Expr) Check(known func(string) bool, suggest func(string) string) error {
	var walk func(n *Node) error
	walk = func(n *Node) error {
		switch n.Op {
		case OpVar:
			if !known(n.Name) {
				msg := fmt.Sprintf("unknown variable %q", n.Name)
				if suggest != nil {
					if s := suggest(n.Name); s != "" {
						msg += fmt.Sprintf(" (did you mean %q?)", s)
					}
				}
				return errorf(n.Pos, "%s", msg)
			}
		case OpCall:
			spec, ok := Funcs[n.Name]
			if !ok {
				names := make([]string, 0, len(Funcs))
				for k := range Funcs {
					names = append(names, k)
				}
				sort.Strings(names)
				return errorf(n.Pos, "unknown function %q (available: %s)", n.Name, strings.Join(names, ", "))
			}
			if len(n.Args) < spec.Min || spec.Max >= 0 && len(n.Args) > spec.Max {
				want := fmt.Sprint(spec.Min)
				switch {
				case spec.Max < 0:
					want += " or more"
				case spec.Max != spec.Min:
					want = fmt.Sprintf("%d to %d", spec.Min, spec.Max)
				}
				return errorf(n.Pos, "%s() takes %s arguments, got %d", n.Name, want, len(n.Args))
			}
		}
		for _, a := range n.Args {
			if err := walk(a); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(e.Root)
}

// MarshalJSON encodes the IR: ["n",1] ["s","x"] ["b",true] ["v","name"]
// ["call","name",arg...] [op,arg...].
func (e *Expr) MarshalJSON() ([]byte, error) {
	return json.Marshal(toIR(e.Root))
}

func toIR(n *Node) []any {
	switch n.Op {
	case OpNum:
		return []any{OpNum, n.Num}
	case OpStr:
		return []any{OpStr, n.Str}
	case OpBool:
		return []any{OpBool, n.Bool}
	case OpVar:
		return []any{OpVar, n.Name}
	case OpCall:
		out := []any{OpCall, n.Name}
		for _, a := range n.Args {
			out = append(out, toIR(a))
		}
		return out
	default:
		out := []any{n.Op}
		for _, a := range n.Args {
			out = append(out, toIR(a))
		}
		return out
	}
}

// UnmarshalJSON decodes the IR produced by MarshalJSON.
func (e *Expr) UnmarshalJSON(b []byte) error {
	var raw any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	n, err := fromIR(raw)
	if err != nil {
		return err
	}
	e.Root = n
	return nil
}

func fromIR(raw any) (*Node, error) {
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil, fmt.Errorf("invalid expression IR: %v", raw)
	}
	op, _ := arr[0].(string)
	n := &Node{Op: op, Pos: -1}
	switch op {
	case OpNum:
		n.Num, _ = arr[1].(float64)
	case OpStr:
		n.Str, _ = arr[1].(string)
	case OpBool:
		n.Bool, _ = arr[1].(bool)
	case OpVar:
		n.Name, _ = arr[1].(string)
	case OpCall:
		n.Name, _ = arr[1].(string)
		for _, a := range arr[2:] {
			c, err := fromIR(a)
			if err != nil {
				return nil, err
			}
			n.Args = append(n.Args, c)
		}
	default:
		for _, a := range arr[1:] {
			c, err := fromIR(a)
			if err != nil {
				return nil, err
			}
			n.Args = append(n.Args, c)
		}
	}
	return n, nil
}
