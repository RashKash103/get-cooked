// Package ingline parses free-text ingredient lines such as
// "2 1/4 cups all-purpose flour, sifted" into amount, unit, name and note.
package ingline

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/RashKash103/get-cooked/internal/units"
)

// Line is a parsed ingredient line.
type Line struct {
	Amount    float64 // 0 when absent
	AmountMax float64 // upper bound of a range ("2-3"), else 0
	Unit      string  // canonical unit id, "" when absent
	Name      string
	Note      string
}

var vulgar = map[rune]float64{
	'½': 0.5, '⅓': 1.0 / 3, '⅔': 2.0 / 3, '¼': 0.25, '¾': 0.75, '⅛': 0.125, '⅜': 0.375,
	'⅝': 0.625, '⅞': 0.875, '⅕': 0.2, '⅖': 0.4, '⅗': 0.6, '⅘': 0.8, '⅙': 1.0 / 6, '⅚': 5.0 / 6,
}

var (
	fracRe = regexp.MustCompile(`^(?:(\d+)\s+)?(\d+)\s*/\s*(\d+)`)
	decRe  = regexp.MustCompile(`^(\d+(?:[.,]\d+)?)\s*([½⅓⅔¼¾⅛⅜⅝⅞⅕⅖⅗⅘⅙⅚])?`)
	vulRe  = regexp.MustCompile(`^[½⅓⅔¼¾⅛⅜⅝⅞⅕⅖⅗⅘⅙⅚]`)
)

// number parses a leading quantity like "2", "2.5", "1 1/2", "1½", "½", "3/4"
// and returns the value and the number of bytes consumed.
func number(s string) (float64, int) {
	atof := func(x string) float64 {
		f, _ := strconv.ParseFloat(strings.ReplaceAll(x, ",", "."), 64)
		return f
	}
	if m := fracRe.FindStringSubmatch(s); m != nil {
		d := atof(m[3])
		if d == 0 {
			return 0, 0
		}
		return atof(m[1]) + atof(m[2])/d, len(m[0])
	}
	if m := decRe.FindStringSubmatch(s); m != nil {
		v := atof(m[1])
		if m[2] != "" {
			v += vulgar[[]rune(m[2])[0]]
		}
		return v, len(m[0])
	}
	if m := vulRe.FindString(s); m != "" {
		return vulgar[[]rune(m)[0]], len(m)
	}
	return 0, 0
}

// Parse parses one ingredient line using the given unit table.
func Parse(s string, tb *units.Table) Line {
	s = strings.TrimSpace(strings.TrimLeft(s, "-*•▢ \t"))
	var l Line
	rest := s
	if v, n := number(rest); n > 0 {
		l.Amount = v
		rest = strings.TrimSpace(rest[n:])
		// ranges: "2-3", "2 to 3", "2–3"
		for _, sep := range []string{"-", "–", "—", "to "} {
			if strings.HasPrefix(rest, sep) {
				if v2, n2 := number(strings.TrimSpace(rest[len(sep):])); n2 > 0 {
					l.AmountMax = v2
					r := strings.TrimSpace(rest[len(sep):])
					rest = strings.TrimSpace(r[n2:])
				}
				break
			}
		}
	}
	// "1 (14 oz) can tomatoes": keep the parenthetical as a note
	var notes []string
	if strings.HasPrefix(rest, "(") {
		if end := strings.Index(rest, ")"); end > 0 {
			notes = append(notes, strings.TrimSpace(rest[1:end]))
			rest = strings.TrimSpace(rest[end+1:])
		}
	}
	if l.Amount > 0 || strings.HasPrefix(strings.ToLower(rest), "pinch") || strings.HasPrefix(strings.ToLower(rest), "dash") {
		fields := strings.Fields(rest)
		// try two-word units first ("fl oz", "fluid ounces")
		for _, n := range []int{2, 1} {
			if len(fields) > n {
				cand := strings.Join(fields[:n], " ")
				if x, ok := tb.Lookup(cand); ok && x.ID != "each" {
					l.Unit = x.ID
					rest = strings.TrimSpace(strings.Join(fields[n:], " "))
					if strings.HasPrefix(rest, "of ") {
						rest = rest[3:]
					}
					break
				}
			}
		}
		if l.Amount == 0 && l.Unit != "" {
			l.Amount = 1
		}
	}
	name := rest
	if i := strings.Index(name, ","); i >= 0 {
		notes = append(notes, strings.TrimSpace(name[i+1:]))
		name = name[:i]
	}
	if i := strings.Index(name, "("); i >= 0 {
		if j := strings.Index(name[i:], ")"); j > 0 {
			notes = append(notes, strings.TrimSpace(name[i+1:i+j]))
			name = name[:i] + name[i+j+1:]
		}
	}
	lower := strings.ToLower(name)
	for _, suffix := range []string{" to taste", " for serving", " for garnish", ", optional", " (optional)", " optional"} {
		if strings.HasSuffix(lower, suffix) {
			notes = append(notes, strings.TrimSpace(strings.TrimPrefix(suffix, ",")))
			name = name[:len(name)-len(suffix)]
			lower = strings.ToLower(name)
		}
	}
	l.Name = strings.Join(strings.Fields(name), " ")
	var clean []string
	for _, n := range notes {
		if n != "" {
			clean = append(clean, n)
		}
	}
	l.Note = strings.Join(clean, ", ")
	return l
}

// Singular makes a best-effort singular form of an English noun phrase's last word.
func Singular(s string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	w := words[len(words)-1]
	lw := strings.ToLower(w)
	switch {
	case strings.HasSuffix(lw, "ies") && len(lw) > 4:
		w = w[:len(w)-3] + "y"
	case strings.HasSuffix(lw, "oes") || strings.HasSuffix(lw, "ches") || strings.HasSuffix(lw, "shes"):
		w = w[:len(w)-2]
	case strings.HasSuffix(lw, "ss") || strings.HasSuffix(lw, "us") || strings.HasSuffix(lw, "sses"):
	case strings.HasSuffix(lw, "s") && len(lw) > 3:
		w = w[:len(w)-1]
	}
	words[len(words)-1] = w
	return strings.Join(words, " ")
}
