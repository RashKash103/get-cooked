package recipe

import (
	"fmt"
	"strings"
)

// Diag is a diagnostic tied to a file position.
type Diag struct {
	File string `json:"file"`
	Line int    `json:"line,omitempty"`
	Col  int    `json:"col,omitempty"`
	Msg  string `json:"message"`
	Warn bool   `json:"warning,omitempty"`
}

func (d Diag) Error() string {
	var sb strings.Builder
	sb.WriteString(d.File)
	if d.Line > 0 {
		fmt.Fprintf(&sb, ":%d", d.Line)
		if d.Col > 0 {
			fmt.Fprintf(&sb, ":%d", d.Col)
		}
	}
	if d.Warn {
		sb.WriteString(": warning: ")
	} else {
		sb.WriteString(": ")
	}
	sb.WriteString(d.Msg)
	return sb.String()
}

// Diags is a list of diagnostics.
type Diags []Diag

// HasErrors reports whether any diagnostic is an error.
func (ds Diags) HasErrors() bool {
	for _, d := range ds {
		if !d.Warn {
			return true
		}
	}
	return false
}

func (ds Diags) Error() string {
	lines := make([]string, len(ds))
	for i, d := range ds {
		lines[i] = d.Error()
	}
	return strings.Join(lines, "\n")
}

// Loc is a source location.
type Loc struct {
	File string
	Line int
	Col  int
}

// Diag makes an error diagnostic at l.
func (l Loc) Diag(format string, args ...any) Diag {
	return Diag{File: l.File, Line: l.Line, Col: l.Col, Msg: fmt.Sprintf(format, args...)}
}

// suggest returns the candidate closest to s (edit distance ≤ 2), or "".
func suggest(s string, candidates []string) string {
	best, bestD := "", 3
	for _, c := range candidates {
		if d := levenshtein(strings.ToLower(s), strings.ToLower(c)); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
