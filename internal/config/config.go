// Package config loads get-cooked.yaml, the site configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/RashKash103/get-cooked/internal/units"
)

// FileName is the site config file name.
const FileName = "get-cooked.yaml"

// Config is the site configuration.
type Config struct {
	Title       string   `yaml:"title" json:"title"`
	Description string   `yaml:"description" json:"description,omitempty"`
	BaseURL     string   `yaml:"base_url" json:"baseUrl"`
	Language    string   `yaml:"language" json:"language"`
	// Languages lists the site's languages; the first is the source language
	// recipes are written in (default: [language]).
	Languages []string `yaml:"languages" json:"languages"`
	Author      string   `yaml:"author" json:"author,omitempty"`
	Recipes     string   `yaml:"recipes" json:"-"`
	Ingredients []string `yaml:"ingredients" json:"-"`
	Static      string   `yaml:"static" json:"-"`
	Theme       Theme    `yaml:"theme" json:"theme"`
	Units       Units    `yaml:"units" json:"-"`
	Timers      Timers   `yaml:"timers" json:"timers"`
	Images      Images   `yaml:"images" json:"-"`

	// Dir is the site root directory (not from YAML).
	Dir string `yaml:"-" json:"-"`
}

// Theme customises the look.
type Theme struct {
	Accent string `yaml:"accent" json:"accent"`
	// CustomCSS is a path (relative to the site root) to extra CSS appended to the bundle.
	CustomCSS string `yaml:"custom_css" json:"-"`
}

// Units customises quantity display.
type Units struct {
	DefaultSystem string                  `yaml:"default_system"`
	Fractions     string                  `yaml:"fractions"`
	Tolerance     float64                 `yaml:"tolerance"`
	Custom        []CustomUnit            `yaml:"custom"`
	Ladders       map[string][]units.Rung `yaml:"ladders"`
	Count         *units.Rule             `yaml:"count"`
}

// CustomUnit declares an extra count unit.
type CustomUnit struct {
	One   string `yaml:"one"`
	Other string `yaml:"other"`
}

// Timers configures how tapping a timer behaves.
type Timers struct {
	// AndroidIntent offers "Open in Clock app" on Android (experimental).
	AndroidIntent bool `yaml:"android_intent" json:"androidIntent"`
	// IOSShortcut names an iOS Shortcut that receives the seconds as input.
	IOSShortcut string `yaml:"ios_shortcut" json:"iosShortcut,omitempty"`
}

// Images configures image processing.
type Images struct {
	// Widths of resized variants to generate for recipe images (0 disables).
	Widths []int `yaml:"widths"`
}

// Default returns the default configuration for dir.
func Default(dir string) *Config {
	return &Config{
		Title:       "Recipes",
		BaseURL:     "/",
		Language:    "en",
		Recipes:     "recipes",
		Ingredients: []string{"ingredients.yaml"},
		Static:      "static",
		Theme:       Theme{Accent: "#ea580c"},
		Units:       Units{DefaultSystem: units.Original, Fractions: "unicode", Tolerance: 0.1},
		Images:      Images{Widths: []int{480, 960, 1600}},
		Dir:         dir,
	}
}

var (
	hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	langRe   = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
)

// Load reads dir/get-cooked.yaml (optional) over the defaults.
func Load(dir string) (*Config, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	c := Default(abs)
	b, err := os.ReadFile(filepath.Join(abs, FileName))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		dec := yaml.NewDecoder(strings.NewReader(string(b)))
		dec.KnownFields(true)
		if err := dec.Decode(c); err != nil && err.Error() != "EOF" {
			return nil, fmt.Errorf("%s: %w", FileName, err)
		}
	}
	if len(c.Languages) == 0 {
		c.Languages = []string{c.Language}
	}
	c.Language = c.Languages[0]
	seen := map[string]bool{}
	for _, l := range c.Languages {
		if !langRe.MatchString(l) {
			return nil, fmt.Errorf("%s: %q is not a language tag like en, es or pt-BR", FileName, l)
		}
		if seen[l] {
			return nil, fmt.Errorf("%s: language %q is listed twice", FileName, l)
		}
		seen[l] = true
	}
	if !strings.HasSuffix(c.BaseURL, "/") {
		c.BaseURL += "/"
	}
	switch c.Units.DefaultSystem {
	case units.Original, units.US, units.Metric:
	default:
		return nil, fmt.Errorf("%s: units.default_system must be original, us or metric", FileName)
	}
	switch c.Units.Fractions {
	case "unicode", "ascii", "decimal":
	default:
		return nil, fmt.Errorf("%s: units.fractions must be unicode, ascii or decimal", FileName)
	}
	if !hexColor.MatchString(c.Theme.Accent) {
		return nil, fmt.Errorf("%s: theme.accent must be a #rrggbb color", FileName)
	}
	return c, nil
}

// UnitTable builds the unit table with the site's overrides applied.
func (c *Config) UnitTable() (*units.Table, error) {
	t := units.Default()
	for _, cu := range c.Units.Custom {
		if cu.One == "" {
			return nil, fmt.Errorf("%s: units.custom entries need 'one'", FileName)
		}
		t.AddCountUnit(cu.One, cu.Other)
	}
	for k, v := range c.Units.Ladders {
		t.Ladders[k] = v
	}
	if c.Units.Count != nil {
		t.Count = *c.Units.Count
	}
	if c.Units.Tolerance > 0 {
		t.Tolerance = c.Units.Tolerance
	}
	t.Fractions = c.Units.Fractions
	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("%s: units: %w", FileName, err)
	}
	return t, nil
}

// Path resolves p relative to the site root.
func (c *Config) Path(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.Dir, p)
}
