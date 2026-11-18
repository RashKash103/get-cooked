package expr

import (
	"math"
	"strconv"
)

// Kind is a value type.
type Kind uint8

// Value kinds.
const (
	KNum Kind = iota
	KStr
	KBool
)

func (k Kind) String() string {
	switch k {
	case KNum:
		return "number"
	case KStr:
		return "string"
	default:
		return "boolean"
	}
}

// Value is a dynamically typed expression value.
type Value struct {
	Kind Kind
	Num  float64
	Str  string
	Bool bool
}

// Num, Str and Bool construct values.
func Num(f float64) Value  { return Value{Kind: KNum, Num: f} }
func Str(s string) Value   { return Value{Kind: KStr, Str: s} }
func Bool(b bool) Value    { return Value{Kind: KBool, Bool: b} }
func (v Value) String() string {
	switch v.Kind {
	case KNum:
		return FormatNumber(v.Num)
	case KStr:
		return v.Str
	default:
		return strconv.FormatBool(v.Bool)
	}
}

// Interface returns the value as a plain Go value (float64, string or bool).
func (v Value) Interface() any {
	switch v.Kind {
	case KNum:
		return v.Num
	case KStr:
		return v.Str
	default:
		return v.Bool
	}
}

// Env resolves identifiers during evaluation.
type Env func(name string) (Value, bool)

// MapEnv builds an Env from a map.
func MapEnv(m map[string]Value) Env {
	return func(name string) (Value, bool) { v, ok := m[name]; return v, ok }
}

// RoundHalfAway rounds to the nearest integer, halves away from zero. The TS
// runtime implements the same rule so both sides agree.
func RoundHalfAway(x float64) float64 { return math.Round(x) }

// RoundTo rounds x to the given number of decimals (half away from zero).
func RoundTo(x float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Round(x*p) / p
}

// FormatNumber prints a number rounded to at most two decimals.
func FormatNumber(x float64) string {
	r := RoundTo(x, 2)
	if r == 0 {
		r = 0 // normalise -0
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// Eval evaluates the expression.
func (e *Expr) Eval(env Env) (Value, error) {
	return eval(e.Root, env)
}

// EvalNum evaluates and requires a finite number.
func (e *Expr) EvalNum(env Env) (float64, error) {
	v, err := e.Eval(env)
	if err != nil {
		return 0, err
	}
	if v.Kind != KNum {
		return 0, errorf(e.Root.Pos, "expected a number but got %s %q", v.Kind, v.String())
	}
	return v.Num, nil
}

// EvalBool evaluates and requires a boolean.
func (e *Expr) EvalBool(env Env) (bool, error) {
	v, err := e.Eval(env)
	if err != nil {
		return false, err
	}
	if v.Kind != KBool {
		return false, errorf(e.Root.Pos, "condition must be true/false but got %s %q", v.Kind, v.String())
	}
	return v.Bool, nil
}

func finite(n *Node, f float64) (Value, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Value{}, errorf(n.Pos, "result is not a finite number")
	}
	return Num(f), nil
}

func evalNum(n *Node, env Env, what string) (float64, error) {
	v, err := eval(n, env)
	if err != nil {
		return 0, err
	}
	if v.Kind != KNum {
		return 0, errorf(n.Pos, "%s expects a number, got %s %q", what, v.Kind, v.String())
	}
	return v.Num, nil
}

func evalBool(n *Node, env Env, what string) (bool, error) {
	v, err := eval(n, env)
	if err != nil {
		return false, err
	}
	if v.Kind != KBool {
		return false, errorf(n.Pos, "%s expects true/false, got %s %q", what, v.Kind, v.String())
	}
	return v.Bool, nil
}

func eval(n *Node, env Env) (Value, error) {
	switch n.Op {
	case OpNum:
		return Num(n.Num), nil
	case OpStr:
		return Str(n.Str), nil
	case OpBool:
		return Bool(n.Bool), nil
	case OpVar:
		v, ok := env(n.Name)
		if !ok {
			return Value{}, errorf(n.Pos, "unknown variable %q", n.Name)
		}
		return v, nil
	case OpNeg:
		a, err := evalNum(n.Args[0], env, "-")
		if err != nil {
			return Value{}, err
		}
		return Num(-a), nil
	case OpNot:
		a, err := evalBool(n.Args[0], env, "!")
		if err != nil {
			return Value{}, err
		}
		return Bool(!a), nil
	case OpCond:
		c, err := evalBool(n.Args[0], env, "?:")
		if err != nil {
			return Value{}, err
		}
		if c {
			return eval(n.Args[1], env)
		}
		return eval(n.Args[2], env)
	case "&&", "||":
		a, err := evalBool(n.Args[0], env, n.Op)
		if err != nil {
			return Value{}, err
		}
		if n.Op == "&&" && !a || n.Op == "||" && a {
			return Bool(a), nil
		}
		b, err := evalBool(n.Args[1], env, n.Op)
		if err != nil {
			return Value{}, err
		}
		return Bool(b), nil
	case "==", "!=":
		a, err := eval(n.Args[0], env)
		if err != nil {
			return Value{}, err
		}
		b, err := eval(n.Args[1], env)
		if err != nil {
			return Value{}, err
		}
		if a.Kind != b.Kind {
			return Value{}, errorf(n.Pos, "cannot compare %s %q with %s %q", a.Kind, a.String(), b.Kind, b.String())
		}
		eq := a == b
		if n.Op == "!=" {
			eq = !eq
		}
		return Bool(eq), nil
	case "+":
		a, err := eval(n.Args[0], env)
		if err != nil {
			return Value{}, err
		}
		b, err := eval(n.Args[1], env)
		if err != nil {
			return Value{}, err
		}
		if a.Kind == KNum && b.Kind == KNum {
			return finite(n, a.Num+b.Num)
		}
		if a.Kind == KStr || b.Kind == KStr {
			return Str(a.String() + b.String()), nil
		}
		return Value{}, errorf(n.Pos, "cannot add %s and %s", a.Kind, b.Kind)
	case "-", "*", "/", "%", "^", "<", "<=", ">", ">=":
		a, err := evalNum(n.Args[0], env, n.Op)
		if err != nil {
			return Value{}, err
		}
		b, err := evalNum(n.Args[1], env, n.Op)
		if err != nil {
			return Value{}, err
		}
		switch n.Op {
		case "-":
			return finite(n, a-b)
		case "*":
			return finite(n, a*b)
		case "/":
			if b == 0 {
				return Value{}, errorf(n.Pos, "division by zero")
			}
			return finite(n, a/b)
		case "%":
			if b == 0 {
				return Value{}, errorf(n.Pos, "modulo by zero")
			}
			return finite(n, math.Mod(a, b))
		case "^":
			return finite(n, math.Pow(a, b))
		case "<":
			return Bool(a < b), nil
		case "<=":
			return Bool(a <= b), nil
		case ">":
			return Bool(a > b), nil
		default:
			return Bool(a >= b), nil
		}
	case OpCall:
		return call(n, env)
	}
	return Value{}, errorf(n.Pos, "invalid expression node %q", n.Op)
}

func call(n *Node, env Env) (Value, error) {
	if spec, ok := Funcs[n.Name]; !ok {
		return Value{}, errorf(n.Pos, "unknown function %q", n.Name)
	} else if len(n.Args) < spec.Min || spec.Max >= 0 && len(n.Args) > spec.Max {
		return Value{}, errorf(n.Pos, "wrong number of arguments to %s()", n.Name)
	}
	switch n.Name {
	case "if":
		c, err := evalBool(n.Args[0], env, "if()")
		if err != nil {
			return Value{}, err
		}
		if c {
			return eval(n.Args[1], env)
		}
		return eval(n.Args[2], env)
	case "str":
		v, err := eval(n.Args[0], env)
		if err != nil {
			return Value{}, err
		}
		return Str(v.String()), nil
	case "plural":
		c, err := evalNum(n.Args[0], env, "plural()")
		if err != nil {
			return Value{}, err
		}
		one, err := eval(n.Args[1], env)
		if err != nil {
			return Value{}, err
		}
		if c == 1 {
			return Str(one.String()), nil
		}
		if len(n.Args) == 3 {
			other, err := eval(n.Args[2], env)
			if err != nil {
				return Value{}, err
			}
			return Str(other.String()), nil
		}
		return Str(one.String() + "s"), nil
	}
	args := make([]float64, len(n.Args))
	for i, a := range n.Args {
		f, err := evalNum(a, env, n.Name+"()")
		if err != nil {
			return Value{}, err
		}
		args[i] = f
	}
	step := func(fn func(float64) float64) (Value, error) {
		if len(args) == 2 {
			if args[1] <= 0 {
				return Value{}, errorf(n.Pos, "%s() step must be positive", n.Name)
			}
			return finite(n, fn(args[0]/args[1])*args[1])
		}
		return finite(n, fn(args[0]))
	}
	switch n.Name {
	case "min", "max":
		r := args[0]
		for _, f := range args[1:] {
			if n.Name == "min" && f < r || n.Name == "max" && f > r {
				r = f
			}
		}
		return Num(r), nil
	case "abs":
		return Num(math.Abs(args[0])), nil
	case "round":
		return step(RoundHalfAway)
	case "floor":
		return step(math.Floor)
	case "ceil":
		return step(math.Ceil)
	case "clamp":
		return Num(math.Min(math.Max(args[0], args[1]), args[2])), nil
	case "sqrt":
		if args[0] < 0 {
			return Value{}, errorf(n.Pos, "sqrt() of a negative number")
		}
		return Num(math.Sqrt(args[0])), nil
	case "pow":
		return finite(n, math.Pow(args[0], args[1]))
	case "lerp":
		return finite(n, args[0]+(args[1]-args[0])*args[2])
	}
	return Value{}, errorf(n.Pos, "unknown function %q", n.Name)
}
