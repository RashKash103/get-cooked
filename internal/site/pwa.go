package site

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // decode JPEG icons
	"image/png"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"

	"github.com/RashKash103/get-cooked/internal/assets"
)

// pwaIcons are the PNG icons every site gets, as (file, size, maskable).
var pwaIcons = []struct {
	file     string
	size     int
	maskable bool
}{
	{"icon-192.png", 192, false},
	{"icon-512.png", 512, false},
	{"icon-maskable-512.png", 512, true},
	{"apple-touch-icon.png", 180, true}, // opaque and full-bleed, like a maskable icon
}

// writeIcons writes the PWA icons. With `icon:` in get-cooked.yaml they are
// resized from that image; otherwise the built-in icons are used. Files
// already present (e.g. from static/) are kept.
func (b *builder) writeIcons() error {
	cfg := b.s.Config
	var src image.Image
	if cfg.Icon != "" {
		f, err := os.ReadFile(cfg.Path(cfg.Icon))
		if err != nil {
			return fmt.Errorf("icon: %w", err)
		}
		img, _, err := image.Decode(bytes.NewReader(f))
		if err != nil {
			return fmt.Errorf("icon %s: %w (use a square PNG or JPEG, at least 512×512)", cfg.Icon, err)
		}
		src = img
	}
	for _, ic := range pwaIcons {
		dst := filepath.Join(b.out, ic.file)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		var data []byte
		var err error
		if src != nil {
			data, err = renderIcon(src, ic.size, ic.maskable)
		} else {
			data, err = fs.ReadFile(assets.FS(), ic.file)
		}
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	// The SVG icon (favicon) only when no custom icon replaces the default.
	if src == nil {
		if _, err := os.Stat(filepath.Join(b.out, "icon.svg")); errors.Is(err, os.ErrNotExist) {
			if svg, err := fs.ReadFile(assets.FS(), "icon.svg"); err == nil {
				if err := os.WriteFile(filepath.Join(b.out, "icon.svg"), svg, 0o644); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// renderIcon scales a square image to size. Maskable icons put the artwork
// at 80% on a full-bleed background (the image's corner colour) so adaptive
// icon masks never clip it.
func renderIcon(src image.Image, size int, maskable bool) ([]byte, error) {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	inner := size
	if maskable {
		bounds := src.Bounds()
		bg := color.RGBAModel.Convert(src.At(bounds.Min.X, bounds.Min.Y)).(color.RGBA)
		if bg.A < 255 {
			bg = color.RGBA{0xfa, 0xfa, 0xf9, 0xff}
		}
		draw.Draw(dst, dst.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)
		inner = size * 8 / 10
	}
	off := (size - inner) / 2
	draw.CatmullRom.Scale(dst, image.Rect(off, off, off+inner, off+inner), src, src.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeManifest writes manifest.webmanifest for installing the site as an app.
func (b *builder) writeManifest() error {
	cfg := b.s.Config
	src := b.s.Langs[0]
	root := b.root
	var icons []map[string]string
	if _, err := os.Stat(filepath.Join(b.out, "icon.svg")); err == nil && cfg.Icon == "" {
		icons = append(icons, map[string]string{"src": root + "icon.svg", "sizes": "any", "type": "image/svg+xml", "purpose": "any"})
	}
	icons = append(icons,
		map[string]string{"src": root + "icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any"},
		map[string]string{"src": root + "icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any"},
		map[string]string{"src": root + "icon-maskable-512.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable"},
	)
	dir := src.Catalog.Dir
	if dir == "" {
		dir = "ltr"
	}
	m := map[string]any{
		"id":               root,
		"name":             cfg.Title,
		"short_name":       cfg.Title,
		"lang":             src.Code,
		"dir":              dir,
		"start_url":        root,
		"scope":            root,
		"display":          "standalone",
		"background_color": "#fafaf9",
		"theme_color":      cfg.Theme.Accent,
		"categories":       []string{"food", "lifestyle"},
		"icons":            icons,
	}
	if cfg.Description != "" {
		m["description"] = cfg.Description
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(b.out, "manifest.webmanifest"), data, 0o644)
}
