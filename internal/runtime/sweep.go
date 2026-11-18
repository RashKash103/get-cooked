package runtime

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/RashKash103/get-cooked/internal/expr"
	"github.com/RashKash103/get-cooked/internal/recipe"
	"github.com/RashKash103/get-cooked/internal/units"
)

// MaxSweepStates bounds the number of option combinations explored.
const MaxSweepStates = 256

// SweepServings returns the servings values explored by Sweep.
func SweepServings(s recipe.ServingsC) []float64 {
	seen := map[float64]bool{}
	var out []float64
	add := func(v float64) {
		v = expr.RoundTo(v, 6)
		if v >= s.Min && v <= s.Max && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	add(s.Min)
	add(s.Default)
	add(s.Max)
	steps := (s.Max - s.Min) / s.Step
	stride := math.Max(1, math.Ceil(steps/24))
	for i := 0.0; i <= steps; i += stride {
		add(s.Min + i*s.Step)
	}
	sort.Float64s(out)
	return out
}

// OptionStates enumerates option combinations: the full cartesian product
// when small, otherwise the defaults plus one-at-a-time variations and the
// all-on / all-off toggle extremes.
func OptionStates(opts []recipe.OptionC) []map[string]any {
	values := make([][]any, len(opts))
	total := 1
	for i, o := range opts {
		switch o.Type {
		case "toggle":
			values[i] = []any{false, true}
		case "choice":
			for _, ch := range o.Choices {
				values[i] = append(values[i], ch.ID)
			}
		default:
			lo, hi := *o.Min, *o.Max
			vals := []any{lo, hi}
			if d, ok := o.Default.(float64); ok && d != lo && d != hi {
				vals = append(vals, d)
			}
			if mid := expr.RoundTo(lo+math.Round((hi-lo)/2 / *o.Step)**o.Step, 6); mid != lo && mid != hi {
				vals = append(vals, mid)
			}
			values[i] = vals
		}
		total *= len(values[i])
		if total > MaxSweepStates {
			total = MaxSweepStates + 1
		}
	}
	defaults := func() map[string]any {
		m := map[string]any{}
		for _, o := range opts {
			m[o.ID] = o.Default
		}
		return m
	}
	if total <= MaxSweepStates {
		out := []map[string]any{{}}
		for i, o := range opts {
			var next []map[string]any
			for _, base := range out {
				for _, v := range values[i] {
					m := map[string]any{}
					for k, x := range base {
						m[k] = x
					}
					m[o.ID] = v
					next = append(next, m)
				}
			}
			out = next
		}
		return out
	}
	out := []map[string]any{defaults()}
	for i, o := range opts {
		for _, v := range values[i] {
			m := defaults()
			m[o.ID] = v
			out = append(out, m)
		}
	}
	allOn, allOff := defaults(), defaults()
	for _, o := range opts {
		if o.Type == "toggle" {
			allOn[o.ID], allOff[o.ID] = true, false
		}
	}
	return append(out, allOn, allOff)
}

// Sweep renders the recipe across many states and reports problems that only
// show up at runtime (type errors, overlapping variants, references to
// excluded ingredients, …). Each distinct problem is reported once.
func Sweep(r *recipe.Recipe) recipe.Diags {
	var diags recipe.Diags
	seen := map[string]bool{}
	for _, opts := range OptionStates(r.C.Options) {
		for _, s := range SweepServings(r.C.Servings) {
			for _, sys := range []string{units.Original, units.US, units.Metric} {
				st := State{Servings: s, Options: opts, System: sys}
				_, err := Render(r.C, st)
				if err == nil {
					continue
				}
				d := recipe.Diag{File: r.Path, Msg: err.Error()}
				if re, ok := err.(*Error); ok && re.Loc.Line > 0 {
					d.Line, d.Col = re.Loc.Line, re.Loc.Col
				}
				key := fmt.Sprintf("%d:%s", d.Line, d.Msg)
				if seen[key] {
					continue
				}
				seen[key] = true
				d.Msg += " [when " + describe(st, r.C) + "]"
				diags = append(diags, d)
			}
		}
	}
	return diags
}

func describe(st State, c *recipe.Compiled) string {
	parts := []string{fmt.Sprintf("servings=%s", expr.FormatNumber(st.Servings))}
	for _, o := range c.Options {
		parts = append(parts, fmt.Sprintf("%s=%v", o.ID, st.Options[o.ID]))
	}
	parts = append(parts, "units="+st.System)
	return strings.Join(parts, ", ")
}
