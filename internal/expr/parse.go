// Package expr implements the get-cooked expression language: a small, safe,
// deterministic language used for amounts, times, conditions and variables.
//
// Expressions are parsed once (in Go) and serialised to a compact JSON IR that
// both the Go and the TypeScript runtimes evaluate.
package expr

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Error is a parse, check or evaluation error with a byte offset into the
// expression source (-1 when unknown).
type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string {
	if e.Pos < 0 {
		return e.Msg
	}
	return fmt.Sprintf("%s (at column %d)", e.Msg, e.Pos+1)
}

func errorf(pos int, format string, args ...any) *Error {
	return &Error{Pos: pos, Msg: fmt.Sprintf(format, args...)}
}

type tokKind int

const (
	tEOF tokKind = iota
	tNum
	tStr
	tIdent
	tOp
)

type token struct {
	kind tokKind
	text string
	num  float64
	pos  int
}

func lex(src string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(src) {
		r, w := utf8.DecodeRuneInString(src[i:])
		switch {
		case unicode.IsSpace(r):
			i += w
		case r >= '0' && r <= '9' || r == '.' && i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9':
			start := i
			for i < len(src) && (src[i] >= '0' && src[i] <= '9' || src[i] == '.' || src[i] == '_') {
				i++
			}
			text := strings.ReplaceAll(src[start:i], "_", "")
			n, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return nil, errorf(start, "invalid number %q", src[start:i])
			}
			toks = append(toks, token{kind: tNum, text: src[start:i], num: n, pos: start})
		case r == '\'' || r == '"':
			start := i
			i++
			var sb strings.Builder
			closed := false
			for i < len(src) {
				c := src[i]
				if c == '\\' && i+1 < len(src) {
					sb.WriteByte(src[i+1])
					i += 2
					continue
				}
				if c == byte(r) {
					closed = true
					i++
					break
				}
				sb.WriteByte(c)
				i++
			}
			if !closed {
				return nil, errorf(start, "unterminated string")
			}
			toks = append(toks, token{kind: tStr, text: sb.String(), pos: start})
		case r == '_' || unicode.IsLetter(r):
			start := i
			for i < len(src) {
				r2, w2 := utf8.DecodeRuneInString(src[i:])
				if r2 == '_' || unicode.IsLetter(r2) || unicode.IsDigit(r2) {
					i += w2
					continue
				}
				break
			}
			toks = append(toks, token{kind: tIdent, text: src[start:i], pos: start})
		default:
			start := i
			two := ""
			if i+1 < len(src) {
				two = src[i : i+2]
			}
			switch two {
			case "==", "!=", "<=", ">=", "&&", "||":
				toks = append(toks, token{kind: tOp, text: two, pos: start})
				i += 2
				continue
			}
			switch r {
			case '+', '-', '*', '/', '%', '^', '<', '>', '!', '?', ':', '(', ')', ',':
				toks = append(toks, token{kind: tOp, text: string(r), pos: start})
				i += w
			case '×':
				toks = append(toks, token{kind: tOp, text: "*", pos: start})
				i += w
			default:
				return nil, errorf(start, "unexpected character %q", r)
			}
		}
	}
	toks = append(toks, token{kind: tEOF, pos: len(src)})
	return toks, nil
}

type parser struct {
	toks []token
	i    int
	src  string
}

// Parse parses an expression.
func Parse(src string) (*Expr, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, src: src}
	if p.peek().kind == tEOF {
		return nil, errorf(0, "empty expression")
	}
	n, err := p.ternary()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, errorf(t.pos, "unexpected %q", t.text)
	}
	return &Expr{Root: n, Src: src}, nil
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) next() token { t := p.toks[p.i]; p.i++; return t }

func (p *parser) isOp(s string) bool {
	t := p.peek()
	return t.kind == tOp && t.text == s
}

func (p *parser) expect(s string) error {
	t := p.peek()
	if t.kind != tOp || t.text != s {
		if t.kind == tEOF {
			return errorf(t.pos, "expected %q but expression ended", s)
		}
		return errorf(t.pos, "expected %q, found %q", s, t.text)
	}
	p.i++
	return nil
}

func (p *parser) ternary() (*Node, error) {
	cond, err := p.binary(0)
	if err != nil {
		return nil, err
	}
	if !p.isOp("?") {
		return cond, nil
	}
	pos := p.next().pos
	a, err := p.ternary()
	if err != nil {
		return nil, err
	}
	if err := p.expect(":"); err != nil {
		return nil, err
	}
	b, err := p.ternary()
	if err != nil {
		return nil, err
	}
	return &Node{Op: OpCond, Args: []*Node{cond, a, b}, Pos: pos}, nil
}

var precedence = [][]string{
	{"||"},
	{"&&"},
	{"==", "!="},
	{"<", "<=", ">", ">="},
	{"+", "-"},
	{"*", "/", "%"},
}

func (p *parser) binary(level int) (*Node, error) {
	if level == len(precedence) {
		return p.unary()
	}
	left, err := p.binary(level + 1)
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != tOp || !contains(precedence[level], t.text) {
			return left, nil
		}
		p.i++
		right, err := p.binary(level + 1)
		if err != nil {
			return nil, err
		}
		left = &Node{Op: t.text, Args: []*Node{left, right}, Pos: t.pos}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (p *parser) unary() (*Node, error) {
	t := p.peek()
	if t.kind == tOp && (t.text == "-" || t.text == "!" || t.text == "+") {
		p.i++
		arg, err := p.unary()
		if err != nil {
			return nil, err
		}
		switch t.text {
		case "-":
			if arg.Op == OpNum {
				return &Node{Op: OpNum, Num: -arg.Num, Pos: t.pos}, nil
			}
			return &Node{Op: OpNeg, Args: []*Node{arg}, Pos: t.pos}, nil
		case "!":
			return &Node{Op: OpNot, Args: []*Node{arg}, Pos: t.pos}, nil
		default:
			return arg, nil
		}
	}
	return p.power()
}

func (p *parser) power() (*Node, error) {
	base, err := p.primary()
	if err != nil {
		return nil, err
	}
	if p.isOp("^") {
		t := p.next()
		exp, err := p.unary() // right associative, allows 2^-1
		if err != nil {
			return nil, err
		}
		return &Node{Op: "^", Args: []*Node{base, exp}, Pos: t.pos}, nil
	}
	return base, nil
}

func (p *parser) primary() (*Node, error) {
	t := p.next()
	switch t.kind {
	case tNum:
		return &Node{Op: OpNum, Num: t.num, Pos: t.pos}, nil
	case tStr:
		return &Node{Op: OpStr, Str: t.text, Pos: t.pos}, nil
	case tIdent:
		switch t.text {
		case "true", "false":
			return &Node{Op: OpBool, Bool: t.text == "true", Pos: t.pos}, nil
		}
		if p.isOp("(") {
			p.i++
			var args []*Node
			if !p.isOp(")") {
				for {
					a, err := p.ternary()
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if p.isOp(",") {
						p.i++
						continue
					}
					break
				}
			}
			if err := p.expect(")"); err != nil {
				return nil, err
			}
			return &Node{Op: OpCall, Name: t.text, Args: args, Pos: t.pos}, nil
		}
		return &Node{Op: OpVar, Name: t.text, Pos: t.pos}, nil
	case tOp:
		if t.text == "(" {
			n, err := p.ternary()
			if err != nil {
				return nil, err
			}
			if err := p.expect(")"); err != nil {
				return nil, err
			}
			return n, nil
		}
		return nil, errorf(t.pos, "unexpected %q", t.text)
	default:
		return nil, errorf(t.pos, "unexpected end of expression")
	}
}
