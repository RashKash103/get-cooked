// Package i18n loads UI translations (i18n/<lang>.yaml in a site) on top of
// the built-in English strings, and recipe translation files
// (<recipe>.<lang>.yaml) that map English source text to translated text.
package i18n

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/RashKash103/get-cooked/internal/units"
)

//go:embed en.yaml
var EnglishYAML []byte

// Default is the language code of the built-in strings.
const Default = "en"

// Label is a singular/plural pair.
type Label struct {
	One   string `yaml:"one" json:"one"`
	Other string `yaml:"other" json:"other"`
}

// IngName overrides an ingredient database entry's display name.
type IngName struct {
	Name   string `yaml:"name"`
	Plural string `yaml:"plural"`
}

// Catalog holds the UI strings for one language.
type Catalog struct {
	Lang        string             `yaml:"-"`
	Name        string             `yaml:"name"`
	Dir         string             `yaml:"dir"`
	Decimal     string             `yaml:"decimal"`
	Of          string             `yaml:"of"`
	System      string             `yaml:"system"`
	Digits      string             `yaml:"digits"`
	Fractions   string             `yaml:"fractions"`
	UI          map[string]string  `yaml:"ui"`
	Times       map[string]string  `yaml:"times"`
	Servings    Label              `yaml:"servings"`
	Duration    map[string]string  `yaml:"duration"`
	Units       map[string]Label   `yaml:"units"`
	Categories  map[string]string  `yaml:"categories"`
	Ingredients map[string]IngName `yaml:"ingredients"`
}

var tagRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// ValidTag reports whether s looks like a BCP 47 language tag ("en", "pt-BR").
func ValidTag(s string) bool { return tagRe.MatchString(s) }

// English returns the built-in catalog.
func English() *Catalog {
	var c Catalog
	if err := yaml.Unmarshal(EnglishYAML, &c); err != nil {
		panic(err)
	}
	c.Lang = Default
	return &c
}

// Path is where a site keeps the catalog for lang.
func Path(siteDir, lang string) string { return filepath.Join(siteDir, "i18n", lang+".yaml") }

// Load returns the catalog for lang: English defaults overlaid with the
// site's i18n/<lang>.yaml (if present). Missing keys fall back to English.
func Load(siteDir, lang string) (*Catalog, error) {
	c := English()
	c.Lang = lang
	if lang != Default {
		c.Name = lang
	}
	b, err := os.ReadFile(Path(siteDir, lang))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	var o Catalog
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&o); err != nil && err.Error() != "EOF" {
		return nil, fmt.Errorf("%s: %w", Path(siteDir, lang), err)
	}
	if o.Name != "" {
		c.Name = o.Name
	}
	if o.Dir != "" {
		c.Dir = o.Dir
	}
	if o.Decimal != "" {
		c.Decimal = o.Decimal
	}
	c.Of = o.Of
	if o.Digits != "" {
		d, err := ResolveDigits(o.Digits)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", Path(siteDir, lang), err)
		}
		c.Digits = d
	}
	switch o.Fractions {
	case "", "unicode", "ascii", "decimal":
		c.Fractions = o.Fractions
	default:
		return nil, fmt.Errorf("%s: fractions must be unicode, ascii or decimal", Path(siteDir, lang))
	}
	switch o.System {
	case "", "original", "us", "metric":
		c.System = o.System
	default:
		return nil, fmt.Errorf("%s: system must be original, us or metric", Path(siteDir, lang))
	}
	merge(c.UI, o.UI)
	merge(c.Times, o.Times)
	merge(c.Duration, o.Duration)
	if c.Categories == nil {
		c.Categories = map[string]string{}
	}
	merge(c.Categories, o.Categories)
	if o.Servings.One != "" {
		c.Servings.One = o.Servings.One
	}
	if o.Servings.Other != "" {
		c.Servings.Other = o.Servings.Other
	}
	if c.Units == nil {
		c.Units = map[string]Label{}
	}
	for k, v := range o.Units {
		c.Units[k] = v
	}
	if c.Ingredients == nil {
		c.Ingredients = map[string]IngName{}
	}
	for k, v := range o.Ingredients {
		c.Ingredients[k] = v
	}
	return c, nil
}

func merge(dst, src map[string]string) {
	for k, v := range src {
		if v != "" {
			dst[k] = v
		}
	}
}

// T translates a UI key, substituting {name} placeholders from pairs.
func (c *Catalog) T(key string, pairs ...string) string {
	s, ok := c.UI[key]
	if !ok {
		s = key
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		s = strings.ReplaceAll(s, "{"+pairs[i]+"}", pairs[i+1])
	}
	return s
}

// TimeLabel returns the label for a times key.
func (c *Catalog) TimeLabel(id string) string {
	if l, ok := c.Times[id]; ok {
		return l
	}
	return strings.ToUpper(id[:1]) + strings.ReplaceAll(id[1:], "_", " ")
}

// Localize returns a copy of the unit table with translated labels,
// duration words and decimal separator.
func (c *Catalog) Localize(t *units.Table) *units.Table {
	cp := *t
	cp.Units = make(map[string]*units.Unit, len(t.Units))
	for id, u := range t.Units {
		x := *u
		if l, ok := c.Units[id]; ok {
			if l.One != "" {
				x.One = l.One
			}
			if l.Other != "" {
				x.Other = l.Other
			} else if l.One != "" {
				x.Other = l.One
			}
		}
		cp.Units[id] = &x
	}
	cp.Ladders = make(map[string][]units.Rung, len(t.Ladders))
	for k, rungs := range t.Ladders {
		rs := make([]units.Rung, len(rungs))
		copy(rs, rungs)
		for i := range rs {
			if l, ok := c.Units[rs[i].Tiny]; ok && l.One != "" {
				rs[i].Tiny = l.One
			}
		}
		cp.Ladders[k] = rs
	}
	cp.Decimal = c.Decimal
	cp.Of = c.Of
	cp.Digits = c.Digits
	if c.Fractions != "" {
		cp.Fractions = c.Fractions
	}
	cp.Words = map[string]string{}
	for k, v := range c.Duration {
		cp.Words[k] = v
	}
	return &cp
}

// NumberingSystems maps CLDR numbering system names to their digits.
var NumberingSystems = map[string]string{
	"latn":     "0123456789",
	"arab":     "٠١٢٣٤٥٦٧٨٩",
	"arabext":  "۰۱۲۳۴۵۶۷۸۹",
	"beng":     "০১২৩৪৫৬৭৮৯",
	"deva":     "०१२३४५६७८९",
	"fullwide": "０１２３４５６７８９",
	"gujr":     "૦૧૨૩૪૫૬૭૮૯",
	"guru":     "੦੧੨੩੪੫੬੭੮੯",
	"khmr":     "០១២៣៤៥៦៧៨៩",
	"knda":     "೦೧೨೩೪೫೬೭೮೯",
	"laoo":     "໐໑໒໓໔໕໖໗໘໙",
	"mlym":     "൦൧൨൩൪൫൬൭൮൯",
	"mymr":     "၀၁၂၃၄၅၆၇၈၉",
	"orya":     "୦୧୨୩୪୫୬୭୮୯",
	"tamldec":  "௦௧௨௩௪௫௬௭௮௯",
	"telu":     "౦౧౨౩౪౫౬౭౮౯",
	"thai":     "๐๑๒๓๔๕๖๗๘๙",
	"tibt":     "༠༡༢༣༤༥༦༧༨༩",
}

// ResolveDigits accepts a numbering system name or ten digit characters.
func ResolveDigits(s string) (string, error) {
	if d, ok := NumberingSystems[s]; ok {
		if s == "latn" {
			return "", nil
		}
		return d, nil
	}
	if len([]rune(s)) == 10 {
		return s, nil
	}
	names := make([]string, 0, len(NumberingSystems))
	for k := range NumberingSystems {
		names = append(names, k)
	}
	sort.Strings(names)
	return "", fmt.Errorf("digits must be a numbering system (%s) or ten characters for 0–9", strings.Join(names, ", "))
}

// Num replaces ASCII digits in s with the language's digits.
func (c *Catalog) Num(s string) string {
	t := units.Table{Digits: c.Digits}
	return t.L(s)
}

// UIKeys returns the UI keys in sorted order.
func (c *Catalog) UIKeys() []string {
	out := make([]string, 0, len(c.UI))
	for k := range c.UI {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
