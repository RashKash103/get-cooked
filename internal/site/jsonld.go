package site

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/RashKash103/get-cooked/internal/recipe"
	"github.com/RashKash103/get-cooked/internal/runtime"
)

// isoDuration formats minutes as an ISO 8601 duration (PT1H5M).
func isoDuration(min float64) string {
	total := int(math.Round(min))
	h, m := total/60, total%60
	s := "PT"
	if h > 0 {
		s += fmt.Sprintf("%dH", h)
	}
	if m > 0 || h == 0 {
		s += fmt.Sprintf("%dM", m)
	}
	return s
}

// jsonLD returns schema.org/Recipe structured data for the default state.
func jsonLD(r *recipe.Recipe, m *runtime.Model, image, url string) string {
	d := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Recipe",
		"name":        r.Title,
		"recipeYield": m.Yield,
	}
	set := func(k, v string) {
		if v != "" {
			d[k] = v
		}
	}
	set("description", r.Description)
	set("image", image)
	set("url", url)
	set("recipeCategory", r.Category)
	set("recipeCuisine", r.Cuisine)
	set("datePublished", r.Date)
	if len(r.Tags) > 0 {
		d["keywords"] = strings.Join(r.Tags, ", ")
	}
	if r.Author != "" {
		d["author"] = map[string]string{"@type": "Person", "name": r.Author}
	}
	var cook float64
	for _, t := range m.Times {
		switch t.ID {
		case "prep":
			d["prepTime"] = isoDuration(t.Minutes)
		case "total":
			d["totalTime"] = isoDuration(t.Minutes)
		default:
			cook += t.Minutes
		}
	}
	if cook > 0 {
		d["cookTime"] = isoDuration(cook)
	}
	var ings []string
	for _, s := range m.Ingredients {
		for _, it := range s.Items {
			line := it.Name
			if it.Q != nil {
				line = it.Q.String() + " " + line
			}
			if it.Note != "" {
				line += ", " + it.Note
			}
			ings = append(ings, line)
		}
	}
	d["recipeIngredient"] = ings
	var steps []map[string]string
	for _, s := range m.Steps {
		for _, st := range s.Items {
			var sb strings.Builder
			for _, seg := range st.Segs {
				if seg.T == "ing" && seg.Q != "" {
					sb.WriteString(seg.Q + " ")
				}
				sb.WriteString(seg.V)
			}
			steps = append(steps, map[string]string{"@type": "HowToStep", "text": sb.String()})
		}
	}
	d["recipeInstructions"] = steps
	b, _ := json.Marshal(d)
	return string(b)
}
