// Package tmpl parses get-cooked text templates: plain text with inline
// expressions ({expr | fmt}), conditionals ({if}…{else}…{end}), ingredient
// references (@id, @id{portion}, @id{-}), equipment references (&id) and
// timers (~{minutes}).
package tmpl

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/RashKash103/get-cooked/internal/expr"
)

// Segment kinds.
const (
	KText  = "text"
	KExpr  = "expr"
	KIng   = "ing"
	KEquip = "eq"
	KTimer = "timer"
	KIf    = "if"
)

// Formatters accepted after a pipe, with their required argument count.
var Formatters = map[string]int{
	"frac":     0,
	"int":      0,
	"duration": 0,
	"temp":     0,
	"temp_c":   0,
	"len":      0,
	"len_cm":   0,
	"qty":      1,
}

// Segment is one piece of a template.
type Segment struct {
	Kind     string
	Text     string     // KText
	Expr     *expr.Expr // KExpr value, KIng portion (nil = full), KTimer minutes
	Fmt      string     // KExpr formatter
	Args     []string   // formatter args
	ID       string     // KIng / KEquip
	NameOnly bool       // KIng: @id{-}
	Branches []Branch   // KIf
	Else     Template   // KIf
	Pos      int
}

// Branch is one {if}/{elif} arm.
type Branch struct {
	Cond *expr.Expr `json:"c"`
	Body Template   `json:"t"`
}

// Template is a parsed text template.
type Template []Segment

// Error is a template error with a byte offset.
type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("%s (at column %d)", e.Msg, e.Pos+1) }

// IsStatic reports whether the template is plain text.
func (t Template) IsStatic() bool {
	for _, s := range t {
		if s.Kind != KText {
			return false
		}
	}
	return true
}

// String returns the plain text of a static template.
func (t Template) String() string {
	var sb strings.Builder
	for _, s := range t {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

// Plain makes a static template from text.
func Plain(s string) Template {
	if s == "" {
		return nil
	}
	return Template{{Kind: KText, Text: s}}
}

type frame struct {
	seg      *Segment
	pos      int
	inElse   bool
	parent   *Template
	children *Template
}

// Parse parses template source.
func Parse(src string) (Template, error) {
	var root Template
	cur := &root
	var stack []*frame
	var text strings.Builder
	textStart := 0
	flush := func() {
		if text.Len() > 0 {
			*cur = append(*cur, Segment{Kind: KText, Text: text.String(), Pos: textStart})
			text.Reset()
		}
	}
	addText := func(pos int, s string) {
		if text.Len() == 0 {
			textStart = pos
		}
		text.WriteString(s)
	}
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\\' && i+1 < len(src) && strings.IndexByte(`{}@&~\`, src[i+1]) >= 0:
			addText(i, src[i+1:i+2])
			i += 2
		case c == '{':
			end, err := closeBrace(src, i)
			if err != nil {
				return nil, err
			}
			inner := strings.TrimSpace(src[i+1 : end])
			innerPos := i + 1 + strings.Index(src[i+1:end], inner)
			switch {
			case strings.HasPrefix(inner, "if ") || strings.HasPrefix(inner, "if("):
				flush()
				cond, err := parseExpr(inner[2:], innerPos+2)
				if err != nil {
					return nil, err
				}
				*cur = append(*cur, Segment{Kind: KIf, Pos: i, Branches: []Branch{{Cond: cond}}})
				seg := &(*cur)[len(*cur)-1]
				f := &frame{seg: seg, pos: i, parent: cur}
				stack = append(stack, f)
				cur = &seg.Branches[0].Body
				f.children = cur
			case strings.HasPrefix(inner, "elif ") || strings.HasPrefix(inner, "elif("):
				flush()
				if len(stack) == 0 || stack[len(stack)-1].inElse {
					return nil, &Error{i, "{elif} without a matching {if}"}
				}
				f := stack[len(stack)-1]
				cond, err := parseExpr(inner[4:], innerPos+4)
				if err != nil {
					return nil, err
				}
				f.seg.Branches = append(f.seg.Branches, Branch{Cond: cond})
				cur = &f.seg.Branches[len(f.seg.Branches)-1].Body
			case inner == "else":
				flush()
				if len(stack) == 0 || stack[len(stack)-1].inElse {
					return nil, &Error{i, "{else} without a matching {if}"}
				}
				f := stack[len(stack)-1]
				f.inElse = true
				cur = &f.seg.Else
			case inner == "end":
				flush()
				if len(stack) == 0 {
					return nil, &Error{i, "{end} without a matching {if}"}
				}
				f := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				cur = f.parent
			default:
				flush()
				seg, err := parseInterp(inner, innerPos)
				if err != nil {
					return nil, err
				}
				seg.Pos = i
				*cur = append(*cur, seg)
			}
			i = end + 1
		case c == '}':
			return nil, &Error{i, `unmatched "}" (write \} for a literal brace)`}
		case c == '~' && i+1 < len(src) && src[i+1] == '{':
			end, err := closeBrace(src, i+1)
			if err != nil {
				return nil, err
			}
			e, err := parseExpr(src[i+2:end], i+2)
			if err != nil {
				return nil, err
			}
			flush()
			*cur = append(*cur, Segment{Kind: KTimer, Expr: e, Pos: i})
			i = end + 1
		case (c == '@' || c == '&') && i+1 < len(src) && isIDStart(src[i+1]):
			j := i + 1
			for j < len(src) && isIDChar(src[j]) {
				j++
			}
			for j > i+1 && src[j-1] == '-' { // "@salt-" → id "salt"
				j--
			}
			id := src[i+1 : j]
			flush()
			if c == '&' {
				*cur = append(*cur, Segment{Kind: KEquip, ID: id, Pos: i})
				i = j
				continue
			}
			seg := Segment{Kind: KIng, ID: id, Pos: i}
			if j < len(src) && src[j] == '{' && !isKeywordBrace(src[j:]) {
				end, err := closeBrace(src, j)
				if err != nil {
					return nil, err
				}
				inner := strings.TrimSpace(src[j+1 : end])
				if inner == "-" {
					seg.NameOnly = true
				} else {
					e, err := parseExpr(src[j+1:end], j+1)
					if err != nil {
						return nil, err
					}
					seg.Expr = e
				}
				j = end + 1
			}
			*cur = append(*cur, seg)
			i = j
		default:
			addText(i, src[i:i+1])
			i++
		}
	}
	flush()
	if len(stack) > 0 {
		return nil, &Error{stack[len(stack)-1].pos, "{if} is missing its {end}"}
	}
	return root, nil
}

// isKeywordBrace reports whether s starts with a control tag such as {end},
// so that "@id{end}" is read as a reference followed by the tag.
func isKeywordBrace(s string) bool {
	end := strings.IndexByte(s, '}')
	if end < 0 {
		return false
	}
	inner := strings.TrimSpace(s[1:end])
	return inner == "end" || inner == "else" || strings.HasPrefix(inner, "elif ") || strings.HasPrefix(inner, "if ")
}

func isIDStart(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_' }
func isIDChar(b byte) bool  { return isIDStart(b) || b >= '0' && b <= '9' || b == '-' }

// closeBrace finds the '}' matching the '{' at open, skipping quoted strings.
func closeBrace(src string, open int) (int, error) {
	var quote byte
	for i := open + 1; i < len(src); i++ {
		c := src[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '{':
			return 0, &Error{i, `unexpected "{" inside braces`}
		case c == '}':
			return i, nil
		}
	}
	return 0, &Error{open, `unclosed "{" (write \{ for a literal brace)`}
}

func parseExpr(src string, offset int) (*expr.Expr, error) {
	e, err := expr.Parse(src)
	if err != nil {
		if pe, ok := err.(*expr.Error); ok {
			return nil, &Error{offset + pe.Pos, pe.Msg}
		}
		return nil, err
	}
	return e, nil
}

// parseInterp parses "expr" or "expr | fmt:arg".
func parseInterp(inner string, offset int) (Segment, error) {
	pipe := -1
	var quote byte
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '|' {
			if i+1 < len(inner) && inner[i+1] == '|' {
				i++
				continue
			}
			pipe = i
		}
	}
	src := inner
	seg := Segment{Kind: KExpr}
	if pipe >= 0 {
		src = inner[:pipe]
		parts := strings.Split(strings.TrimSpace(inner[pipe+1:]), ":")
		seg.Fmt = strings.TrimSpace(parts[0])
		for _, a := range parts[1:] {
			seg.Args = append(seg.Args, strings.TrimSpace(a))
		}
		n, ok := Formatters[seg.Fmt]
		if !ok {
			names := make([]string, 0, len(Formatters))
			for k := range Formatters {
				names = append(names, k)
			}
			return seg, &Error{offset + pipe, fmt.Sprintf("unknown formatter %q (available: %s)", seg.Fmt, strings.Join(sortStrings(names), ", "))}
		}
		if len(seg.Args) != n {
			return seg, &Error{offset + pipe, fmt.Sprintf("formatter %q takes %d argument(s), got %d", seg.Fmt, n, len(seg.Args))}
		}
	}
	e, err := parseExpr(src, offset)
	if err != nil {
		return seg, err
	}
	seg.Expr = e
	return seg, nil
}

func sortStrings(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s
}

// Walk visits every segment (depth first), including nested conditionals.
func (t Template) Walk(fn func(s *Segment) error) error {
	for i := range t {
		s := &t[i]
		if err := fn(s); err != nil {
			return err
		}
		if s.Kind == KIf {
			for _, b := range s.Branches {
				if err := b.Body.Walk(fn); err != nil {
					return err
				}
			}
			if err := s.Else.Walk(fn); err != nil {
				return err
			}
		}
	}
	return nil
}

// Exprs returns all expressions in the template with their positions.
func (t Template) Exprs() []*expr.Expr {
	var out []*expr.Expr
	_ = t.Walk(func(s *Segment) error {
		if s.Expr != nil {
			out = append(out, s.Expr)
		}
		for _, b := range s.Branches {
			out = append(out, b.Cond)
		}
		return nil
	})
	return out
}

// MarshalJSON encodes a template as an array where plain text is a string and
// everything else is an object.
func (t Template) MarshalJSON() ([]byte, error) {
	out := make([]any, 0, len(t))
	for _, s := range t {
		switch s.Kind {
		case KText:
			out = append(out, s.Text)
		case KExpr:
			m := map[string]any{"e": s.Expr}
			if s.Fmt != "" {
				m["f"] = s.Fmt
			}
			if len(s.Args) > 0 {
				m["a"] = s.Args
			}
			out = append(out, m)
		case KIng:
			m := map[string]any{"ing": s.ID}
			if s.Expr != nil {
				m["p"] = s.Expr
			}
			if s.NameOnly {
				m["n"] = true
			}
			out = append(out, m)
		case KEquip:
			out = append(out, map[string]any{"eq": s.ID})
		case KTimer:
			out = append(out, map[string]any{"timer": s.Expr})
		case KIf:
			m := map[string]any{"if": s.Branches}
			if len(s.Else) > 0 {
				m["else"] = s.Else
			}
			out = append(out, m)
		}
	}
	return json.Marshal(out)
}

// UnmarshalJSON decodes the format written by MarshalJSON.
func (t *Template) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*t = nil
	for _, r := range raw {
		var s string
		if json.Unmarshal(r, &s) == nil {
			*t = append(*t, Segment{Kind: KText, Text: s})
			continue
		}
		var m struct {
			E     *expr.Expr `json:"e"`
			F     string     `json:"f"`
			A     []string   `json:"a"`
			Ing   *string    `json:"ing"`
			P     *expr.Expr `json:"p"`
			N     bool       `json:"n"`
			Eq    *string    `json:"eq"`
			Timer *expr.Expr `json:"timer"`
			If    []Branch   `json:"if"`
			Else  Template   `json:"else"`
		}
		if err := json.Unmarshal(r, &m); err != nil {
			return err
		}
		switch {
		case m.Ing != nil:
			*t = append(*t, Segment{Kind: KIng, ID: *m.Ing, Expr: m.P, NameOnly: m.N})
		case m.Eq != nil:
			*t = append(*t, Segment{Kind: KEquip, ID: *m.Eq})
		case m.Timer != nil:
			*t = append(*t, Segment{Kind: KTimer, Expr: m.Timer})
		case m.If != nil:
			*t = append(*t, Segment{Kind: KIf, Branches: m.If, Else: m.Else})
		case m.E != nil:
			*t = append(*t, Segment{Kind: KExpr, Expr: m.E, Fmt: m.F, Args: m.A})
		default:
			return fmt.Errorf("invalid template segment %s", r)
		}
	}
	return nil
}
