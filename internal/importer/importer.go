// Package importer extracts schema.org Recipe data (JSON-LD) from a web page
// and turns it into a draft get-cooked recipe. The draft is a starting point:
// the import-recipe Claude skill (or a human) adds ids, references and logic.
package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Recipe is the subset of schema.org/Recipe we use.
type Recipe struct {
	Name         string
	Description  string
	Image        string
	Yield        float64
	YieldUnit    string
	Prep, Cook   float64 // minutes
	Total        float64
	Category     string
	Cuisine      string
	Keywords     []string
	Author       string
	Ingredients  []string
	Instructions []Section
	URL          string
}

// Section is a group of instruction steps (Name may be empty).
type Section struct {
	Name  string
	Steps []string
}

// UserAgent identifies the importer to websites.
const UserAgent = "get-cooked-importer/1.0 (+https://github.com/RashKash103/get-cooked)"

// Fetch downloads a page (max 5 MB).
func Fetch(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 5<<20))
}

var ldRe = regexp.MustCompile(`(?is)<script[^>]+type=["']?application/ld\+json["']?[^>]*>(.*?)</script>`)

// ErrNoRecipe means the page has no schema.org Recipe data.
var ErrNoRecipe = errors.New("no schema.org Recipe found in the page (it may not publish structured data)")

// Extract finds the first Recipe in the page's JSON-LD.
func Extract(page []byte) (*Recipe, error) {
	for _, m := range ldRe.FindAllSubmatch(page, -1) {
		var v any
		if err := json.Unmarshal([]byte(strings.TrimSpace(string(m[1]))), &v); err != nil {
			continue
		}
		if r := find(v); r != nil {
			return parse(r), nil
		}
	}
	return nil, ErrNoRecipe
}

func isRecipe(m map[string]any) bool {
	switch t := m["@type"].(type) {
	case string:
		return t == "Recipe"
	case []any:
		for _, x := range t {
			if x == "Recipe" {
				return true
			}
		}
	}
	return false
}

func find(v any) map[string]any {
	switch x := v.(type) {
	case map[string]any:
		if isRecipe(x) {
			return x
		}
		for _, k := range []string{"@graph", "mainEntity", "itemListElement"} {
			if r := find(x[k]); r != nil {
				return r
			}
		}
	case []any:
		for _, e := range x {
			if r := find(e); r != nil {
				return r
			}
		}
	}
	return nil
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

func clean(s string) string {
	s = html.UnescapeString(tagRe.ReplaceAllString(s, " "))
	return strings.Join(strings.Fields(s), " ")
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return clean(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		if len(x) > 0 {
			return str(x[0])
		}
	case map[string]any:
		for _, k := range []string{"name", "url", "text", "@id"} {
			if s := str(x[k]); s != "" {
				return s
			}
		}
	}
	return ""
}

var isoRe = regexp.MustCompile(`(?i)^P(?:(\d+(?:\.\d+)?)D)?T?(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?$`)

// Minutes parses an ISO 8601 duration such as PT1H30M.
func Minutes(s string) float64 {
	m := isoRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	f := func(i int) float64 { v, _ := strconv.ParseFloat(m[i], 64); return v }
	return f(1)*1440 + f(2)*60 + f(3) + math.Round(f(4)/60)
}

var yieldRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(.*)`)

func parse(m map[string]any) *Recipe {
	r := &Recipe{
		Name:        str(m["name"]),
		Description: str(m["description"]),
		Image:       str(m["image"]),
		Category:    str(m["recipeCategory"]),
		Cuisine:     str(m["recipeCuisine"]),
		Author:      str(m["author"]),
		URL:         str(m["url"]),
		Prep:        Minutes(str(m["prepTime"])),
		Cook:        Minutes(str(m["cookTime"])),
		Total:       Minutes(str(m["totalTime"])),
	}
	// recipeYield: "4", 4, "Makes 24 cookies", ["4", "4 servings"]
	var yields []string
	switch y := m["recipeYield"].(type) {
	case []any:
		for _, e := range y {
			yields = append(yields, str(e))
		}
	default:
		yields = []string{str(y)}
	}
	for _, y := range yields {
		if mm := yieldRe.FindStringSubmatch(y); mm != nil {
			r.Yield, _ = strconv.ParseFloat(mm[1], 64)
			unit := strings.ToLower(strings.TrimSpace(mm[2]))
			if unit != "" && unit != "servings" && unit != "serving" && r.YieldUnit == "" {
				r.YieldUnit = unit
			}
			break
		}
	}
	switch k := m["keywords"].(type) {
	case string:
		for _, w := range strings.Split(k, ",") {
			if w = clean(w); w != "" {
				r.Keywords = append(r.Keywords, strings.ToLower(w))
			}
		}
	case []any:
		for _, w := range k {
			r.Keywords = append(r.Keywords, strings.ToLower(str(w)))
		}
	}
	if ings, ok := m["recipeIngredient"].([]any); ok {
		for _, i := range ings {
			if s := str(i); s != "" {
				r.Ingredients = append(r.Ingredients, s)
			}
		}
	}
	r.Instructions = instructions(m["recipeInstructions"])
	return r
}

func instructions(v any) []Section {
	var out []Section
	cur := Section{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			for _, line := range strings.Split(x, "\n") {
				if s := clean(line); s != "" {
					cur.Steps = append(cur.Steps, s)
				}
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			if x["@type"] == "HowToSection" {
				if len(cur.Steps) > 0 {
					out = append(out, cur)
				}
				cur = Section{Name: str(x["name"])}
				walk(x["itemListElement"])
				out = append(out, cur)
				cur = Section{}
				return
			}
			if t := str(x["text"]); t != "" {
				cur.Steps = append(cur.Steps, t)
			} else if n := str(x["name"]); n != "" {
				cur.Steps = append(cur.Steps, n)
			}
		}
	}
	walk(v)
	if len(cur.Steps) > 0 {
		out = append(out, cur)
	}
	return out
}

// YAML renders a draft get-cooked recipe.
func (r *Recipe) YAML(sourceURL string) ([]byte, error) {
	if r.URL == "" {
		r.URL = sourceURL
	}
	n := func(kv ...any) *yaml.Node {
		m := &yaml.Node{Kind: yaml.MappingNode}
		for i := 0; i+1 < len(kv); i += 2 {
			v := kv[i+1]
			var vn yaml.Node
			if err := vn.Encode(v); err != nil {
				continue
			}
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: kv[i].(string)}, &vn)
		}
		return m
	}
	doc := &yaml.Node{Kind: yaml.MappingNode}
	add := func(k string, v any) {
		var vn yaml.Node
		switch x := v.(type) {
		case *yaml.Node:
			vn = *x
		default:
			if err := vn.Encode(v); err != nil {
				return
			}
		}
		doc.Content = append(doc.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &vn)
	}
	add("title", r.Name)
	if r.Description != "" {
		add("description", r.Description)
	}
	if r.Image != "" {
		add("image", r.Image)
	}
	if r.Category != "" {
		add("category", r.Category)
	}
	if len(r.Keywords) > 0 {
		add("tags", r.Keywords)
	}
	if r.Cuisine != "" {
		add("cuisine", r.Cuisine)
	}
	if r.Author != "" {
		add("author", r.Author)
	}
	src := n("url", r.URL)
	add("source", src)
	add("date", time.Now().Format("2006-01-02"))
	yield := r.Yield
	if yield <= 0 {
		yield = 4
	}
	if r.YieldUnit != "" {
		unit := strings.Fields(r.YieldUnit)
		add("servings", n("default", yield, "unit", strings.TrimSuffix(unit[len(unit)-1], "s")))
	} else {
		add("servings", yield)
	}
	times := &yaml.Node{Kind: yaml.MappingNode}
	addTime := func(k string, v float64) {
		if v > 0 {
			times.Content = append(times.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(int(math.Round(v)))})
		}
	}
	addTime("prep", r.Prep)
	addTime("cook", r.Cook)
	if r.Total > 0 && r.Total != r.Prep+r.Cook {
		addTime("total", r.Total)
	}
	if len(times.Content) > 0 {
		add("times", times)
	}
	if len(r.Ingredients) == 0 {
		r.Ingredients = []string{"TODO"}
	}
	add("ingredients", r.Ingredients)
	var steps []any
	for _, s := range r.Instructions {
		if s.Name == "" {
			for _, st := range s.Steps {
				steps = append(steps, st)
			}
			continue
		}
		steps = append(steps, map[string]any{"section": s.Name, "steps": s.Steps})
	}
	if len(steps) == 0 {
		steps = []any{"TODO"}
	}
	add("steps", steps)
	root := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{doc},
		HeadComment: "yaml-language-server: $schema=https://raw.githubusercontent.com/RashKash103/get-cooked/main/schema/recipe.schema.json\n\n" +
			"Draft imported by `get-cooked import`. Next: give ingredients ids and database keys,\n" +
			"reference them in steps (@id), add timers (~{min}) and scaling logic. See `get-cooked docs format`."}
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	return []byte(sb.String()), enc.Close()
}
