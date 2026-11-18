package site

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/image/draw"
)

type processedImage struct {
	src    string // file name of the default image (relative to the page)
	srcset string // "a-480.jpg 480w, a-960.jpg 960w"
}

type imageProcessor struct {
	widths []int
	cache  string
}

func newImageProcessor(widths []int) *imageProcessor {
	var ws []int
	for _, w := range widths {
		if w > 0 {
			ws = append(ws, w)
		}
	}
	sort.Ints(ws)
	p := &imageProcessor{widths: ws}
	if dir, err := os.UserCacheDir(); err == nil {
		p.cache = filepath.Join(dir, "get-cooked", "images")
	}
	return p
}

// process copies an image next to the page and, for JPEG/PNG sources,
// generates resized variants for responsive srcset.
func (p *imageProcessor) process(src, fromDir, outDir string) (processedImage, error) {
	if isRemote(src) {
		return processedImage{src: src}, nil
	}
	in := filepath.Join(fromDir, src)
	b, err := os.ReadFile(in)
	if err != nil {
		return processedImage{}, err
	}
	name := filepath.Base(src)
	if err := os.WriteFile(filepath.Join(outDir, name), b, 0o644); err != nil {
		return processedImage{}, err
	}
	ext := strings.ToLower(filepath.Ext(name))
	if len(p.widths) == 0 || (ext != ".jpg" && ext != ".jpeg" && ext != ".png") {
		return processedImage{src: name}, nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return processedImage{}, fmt.Errorf("%s: %w", src, err)
	}
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	sum := sha256.Sum256(b)
	key := hex.EncodeToString(sum[:8])
	var decoded image.Image
	var set []string
	for _, w := range p.widths {
		if w >= cfg.Width {
			break
		}
		outName := fmt.Sprintf("%s-%d%s", stem, w, ext)
		data, ok := p.cached(key, w, ext)
		if !ok {
			if decoded == nil {
				decoded, _, err = image.Decode(bytes.NewReader(b))
				if err != nil {
					return processedImage{}, fmt.Errorf("%s: %w", src, err)
				}
			}
			data, err = resize(decoded, w, ext)
			if err != nil {
				return processedImage{}, err
			}
			p.store(key, w, ext, data)
		}
		if err := os.WriteFile(filepath.Join(outDir, outName), data, 0o644); err != nil {
			return processedImage{}, err
		}
		set = append(set, fmt.Sprintf("%s %dw", outName, w))
	}
	set = append(set, fmt.Sprintf("%s %dw", name, cfg.Width))
	return processedImage{src: name, srcset: strings.Join(set, ", ")}, nil
}

func resize(img image.Image, w int, ext string) ([]byte, error) {
	b := img.Bounds()
	h := b.Dy() * w / b.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	var buf bytes.Buffer
	var err error
	if ext == ".png" {
		err = png.Encode(&buf, dst)
	} else {
		err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82})
	}
	return buf.Bytes(), err
}

func (p *imageProcessor) cachePath(key string, w int, ext string) string {
	return filepath.Join(p.cache, fmt.Sprintf("%s-%d%s", key, w, ext))
}

func (p *imageProcessor) cached(key string, w int, ext string) ([]byte, bool) {
	if p.cache == "" {
		return nil, false
	}
	b, err := os.ReadFile(p.cachePath(key, w, ext))
	return b, err == nil
}

func (p *imageProcessor) store(key string, w int, ext string, data []byte) {
	if p.cache == "" {
		return
	}
	if os.MkdirAll(p.cache, 0o755) == nil {
		_ = os.WriteFile(p.cachePath(key, w, ext), data, 0o644)
	}
}
