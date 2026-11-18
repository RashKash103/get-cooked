package i18n

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// A recipe translation lives next to the recipe as <name>.<lang>.yaml and maps
// each English (source) string to its translation:
//
//	strings:
//	  Cranberry Baked Oatmeal: Avena horneada con arándanos
//	  "Heat the oven to {375 | temp}.": "Precalienta el horno a {375 | temp}."
//
// Empty or missing entries fall back to the source text, so a partially
// translated recipe still works.

var translationRe = regexp.MustCompile(`\.([a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*)\.ya?ml$`)

// TranslationLang returns the language of a translation file name, or "".
func TranslationLang(path string) string {
	m := translationRe.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return ""
	}
	return m[1]
}

// TranslationPath returns where the lang translation of a recipe lives.
func TranslationPath(recipePath, lang string) string {
	ext := filepath.Ext(recipePath)
	return strings.TrimSuffix(recipePath, ext) + "." + lang + ext
}

// RecipeTranslation is a loaded translation file.
type RecipeTranslation struct {
	Path    string
	Lang    string
	Strings map[string]string
	Order   []string
}

// LoadRecipeTranslation reads a translation file; it returns nil, nil when the
// file does not exist.
func LoadRecipeTranslation(path, lang string) (*RecipeTranslation, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t := &RecipeTranslation{Path: path, Lang: lang, Strings: map[string]string{}}
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(root.Content) == 0 {
		return t, nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: expected a mapping with a `strings:` key", path)
	}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value != "strings" {
			return nil, fmt.Errorf("%s:%d: unknown key %q (expected `strings:`)", path, doc.Content[i].Line, doc.Content[i].Value)
		}
		m := doc.Content[i+1]
		if m.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s:%d: `strings:` must map source text to translations", path, m.Line)
		}
		for j := 0; j+1 < len(m.Content); j += 2 {
			k, v := m.Content[j].Value, m.Content[j+1].Value
			if _, dup := t.Strings[k]; !dup {
				t.Order = append(t.Order, k)
			}
			t.Strings[k] = v
		}
	}
	return t, nil
}

// Recorder wraps a translation (which may be nil) and records every source
// string looked up, so coverage can be reported and files extracted.
type Recorder struct {
	T       *RecipeTranslation
	Sources []string
	seen    map[string]bool
}

// NewRecorder creates a recorder over t (nil = no translation).
func NewRecorder(t *RecipeTranslation) *Recorder {
	return &Recorder{T: t, seen: map[string]bool{}}
}

var codeRe = regexp.MustCompile(`~?\{[^{}]*\}|[@&][A-Za-z_][A-Za-z0-9_.-]*|[\d\s\p{P}\p{S}]+`)

// translatable reports whether s contains words outside of expressions and
// references ("{pan}" or "@salt" alone has nothing to translate).
func translatable(s string) bool {
	return strings.TrimSpace(codeRe.ReplaceAllString(s, "")) != ""
}

// Tr returns the translation of s, or s itself.
func (r *Recorder) Tr(s string) string {
	if !translatable(s) {
		return s
	}
	if !r.seen[s] {
		r.seen[s] = true
		r.Sources = append(r.Sources, s)
	}
	if r.T != nil {
		if v := r.T.Strings[s]; v != "" {
			return v
		}
	}
	return s
}

// Missing returns source strings without a translation.
func (r *Recorder) Missing() []string {
	var out []string
	for _, s := range r.Sources {
		if r.T == nil || r.T.Strings[s] == "" {
			out = append(out, s)
		}
	}
	return out
}

// Stale returns translated keys that no longer appear in the recipe.
func (r *Recorder) Stale() []string {
	if r.T == nil {
		return nil
	}
	var out []string
	for _, k := range r.T.Order {
		if !r.seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// WriteTranslation writes (or rewrites) a translation file containing every
// source string in order, keeping existing translations. Stale entries are
// kept at the end unless prune is set.
func WriteTranslation(path, lang, recipeFile string, sources []string, existing *RecipeTranslation, prune bool) error {
	keys := &yaml.Node{Kind: yaml.MappingNode}
	have := map[string]bool{}
	add := func(k, v, comment string) {
		kn := &yaml.Node{Kind: yaml.ScalarNode, Value: k, HeadComment: comment}
		vn := &yaml.Node{Kind: yaml.ScalarNode, Value: v}
		if strings.ContainsAny(k, ":#{}[]&*!|>'\"%@`,") || strings.HasPrefix(k, "-") || k != strings.TrimSpace(k) {
			kn.Style = yaml.DoubleQuotedStyle
		}
		if v == "" || strings.ContainsAny(v, ":#{}[]&*!|>'\"%@`,") || strings.HasPrefix(v, "-") {
			vn.Style = yaml.DoubleQuotedStyle
		}
		keys.Content = append(keys.Content, kn, vn)
	}
	for _, s := range sources {
		have[s] = true
		v := ""
		if existing != nil {
			v = existing.Strings[s]
		}
		add(s, v, "")
	}
	if existing != nil && !prune {
		first := true
		for _, k := range existing.Order {
			if have[k] {
				continue
			}
			c := ""
			if first {
				c = "No longer used by the recipe (remove with --prune):"
				first = false
			}
			add(k, existing.Strings[k], c)
		}
	}
	doc := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "strings"}, keys,
	}}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# %s translation of %s.\n", lang, filepath.Base(recipeFile))
	buf.WriteString("# Keys are the original text; translate the values. Empty values fall back to the\n")
	buf.WriteString("# original. Keep {…} expressions, @ingredient, &equipment and ~{timer} references\n")
	buf.WriteString("# unchanged (you may move them). Regenerate with: get-cooked i18n extract\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
