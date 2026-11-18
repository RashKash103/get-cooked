package site

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/RashKash103/get-cooked/internal/assets"
	"github.com/RashKash103/get-cooked/internal/config"
	"github.com/RashKash103/get-cooked/internal/recipe"
	"github.com/RashKash103/get-cooked/internal/runtime"
)

//go:embed templates/*.html
var templateFS embed.FS

// BuildOptions controls a build.
type BuildOptions struct {
	Out     string
	BaseURL string // overrides config base_url when set
	// LiveReload injects a script that reloads the page on rebuild (serve).
	LiveReload bool
}

// Assets are the fingerprinted asset paths (relative to the base).
type Assets struct {
	JS  string
	CSS string
}

type page struct {
	Site        *config.Config
	Base        string
	Title       string
	Description string
	Canonical   string
	OGImage     string
	JSONLD      template.JS
	BodyClass   string
	Accent      template.CSS
	Assets      Assets
	Recipe      *recipeView
	Index       *indexView
}

type recipeView struct {
	R           *recipe.Recipe
	M           *runtime.Model
	Image       string
	SrcSet      string
	ImageAlt    string
	Times       template.HTML
	Equipment   template.HTML
	Ingredients template.HTML
	Steps       template.HTML
	Warnings    template.HTML
	Tips        template.HTML
	Data        template.JS
}

type card struct {
	Title, URL, Image, SrcSet, Category, Time, Search, Initial string
}

type indexView struct {
	Cards      []card
	Categories []string
}

// Build writes the site to opts.Out.
func Build(s *Site, opts BuildOptions) error {
	if !assets.Built() {
		return errors.New("the frontend bundle is missing from this binary; run `pnpm install && pnpm build` before `go build`")
	}
	cfg := s.Config
	baseURL := cfg.BaseURL
	if opts.BaseURL != "" {
		baseURL = opts.BaseURL
		if !strings.HasSuffix(baseURL, "/") {
			baseURL += "/"
		}
	}
	base := baseURL
	absolute := ""
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		base = u.Path
		if base == "" {
			base = "/"
		}
		absolute = strings.TrimSuffix(baseURL, "/")
		absolute = strings.TrimSuffix(absolute, strings.TrimSuffix(base, "/"))
	}
	out := opts.Out
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	tpl, err := template.New("").ParseFS(templateFS, "templates/base.html")
	if err != nil {
		return err
	}
	parse := func(name string) (*template.Template, error) {
		t, err := tpl.Clone()
		if err != nil {
			return nil, err
		}
		return t.ParseFS(templateFS, "templates/"+name)
	}
	recipeTpl, err := parse("recipe.html")
	if err != nil {
		return err
	}
	indexTpl, err := parse("index.html")
	if err != nil {
		return err
	}
	notFoundTpl, err := parse("404.html")
	if err != nil {
		return err
	}

	as, err := writeAssets(cfg.Path(cfg.Theme.CustomCSS), cfg.Theme.CustomCSS != "", out, opts.LiveReload)
	if err != nil {
		return err
	}
	if err := copyDir(cfg.Path(cfg.Static), out); err != nil {
		return err
	}
	newPage := func(title, desc, canonical string) page {
		return page{
			Site: cfg, Base: base, Title: title, Description: desc, Accent: template.CSS(cfg.Theme.Accent),
			Assets: as, Canonical: canonical,
		}
	}
	abs := func(p string) string {
		if absolute == "" {
			return ""
		}
		return absolute + p
	}

	imgs := newImageProcessor(cfg.Images.Widths)
	idx := &indexView{}
	cats := map[string]bool{}
	var sitemap []string
	for _, r := range s.Recipes {
		dir := filepath.Join(out, "recipes", r.Slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		m, err := runtime.Render(r.C, runtime.State{})
		if err != nil {
			return fmt.Errorf("%s: %w", r.Path, err)
		}
		v := &recipeView{
			R: r, M: m, ImageAlt: r.ImageAlt,
			Times:       template.HTML(renderTimes(m)),
			Equipment:   template.HTML(renderEquipment(m)),
			Ingredients: template.HTML(renderIngredients(m, base)),
			Steps:       template.HTML(renderSteps(m, base)),
			Warnings:    template.HTML(renderNotes(m, true)),
			Tips:        template.HTML(renderNotes(m, false)),
		}
		if v.ImageAlt == "" {
			v.ImageAlt = r.Title
		}
		pageURL := base + "recipes/" + r.Slug + "/"
		if r.Image != "" {
			img, err := imgs.process(r.Image, filepath.Dir(r.Path), dir)
			if err != nil {
				return fmt.Errorf("%s: image: %w", r.Path, err)
			}
			if img.src != "" && !isRemote(img.src) {
				v.Image = pageURL + img.src
				v.SrcSet = prefixSrcSet(pageURL, img.srcset)
			} else {
				v.Image = img.src
			}
		}
		// Step images live next to the recipe as well.
		for _, sec := range r.C.Steps {
			for _, st := range sec.Items {
				if st.Image != "" && !isRemote(st.Image) {
					if err := copyFile(filepath.Join(filepath.Dir(r.Path), st.Image), filepath.Join(dir, filepath.Base(st.Image))); err != nil {
						return fmt.Errorf("%s: step image: %w", r.Path, err)
					}
				}
			}
		}
		data, err := pageData(r)
		if err != nil {
			return err
		}
		v.Data = template.JS(data)
		p := newPage(r.Title+" · "+cfg.Title, r.Description, abs(pageURL))
		p.Recipe = v
		p.BodyClass = "page-recipe"
		if v.Image != "" {
			if isRemote(v.Image) {
				p.OGImage = v.Image
			} else {
				p.OGImage = abs(v.Image)
			}
		}
		p.JSONLD = template.JS(jsonLD(r, m, p.OGImage, abs(pageURL)))
		if err := writeTemplate(recipeTpl, filepath.Join(dir, "index.html"), p); err != nil {
			return err
		}
		sitemap = append(sitemap, pageURL)

		var search []string
		search = append(search, r.Title, r.Category, r.Cuisine, r.Description)
		search = append(search, r.Tags...)
		for _, s := range m.Ingredients {
			for _, it := range s.Items {
				search = append(search, it.Name)
			}
		}
		c := card{
			Title: r.Title, URL: pageURL, Category: r.Category, Image: v.Image, SrcSet: v.SrcSet,
			Search:  strings.ToLower(strings.Join(search, " ")),
			Initial: strings.ToUpper(string([]rune(r.Title)[:1])),
		}
		for _, t := range m.Times {
			if t.ID == "total" || len(m.Times) == 1 {
				c.Time = t.Text
			}
		}
		idx.Cards = append(idx.Cards, c)
		if r.Category != "" {
			cats[r.Category] = true
		}
	}
	for c := range cats {
		idx.Categories = append(idx.Categories, c)
	}
	sort.Strings(idx.Categories)

	ip := newPage(cfg.Title, cfg.Description, abs(base))
	ip.Index = idx
	ip.BodyClass = "page-index"
	if err := writeTemplate(indexTpl, filepath.Join(out, "index.html"), ip); err != nil {
		return err
	}
	np := newPage("Not found · "+cfg.Title, "", "")
	np.BodyClass = "page-404"
	if err := writeTemplate(notFoundTpl, filepath.Join(out, "404.html"), np); err != nil {
		return err
	}
	if err := writeMeta(out, cfg.Title, base, absolute, append([]string{base}, sitemap...), as); err != nil {
		return err
	}
	return nil
}

func isRemote(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "//")
}

func prefixSrcSet(prefix, srcset string) string {
	if srcset == "" {
		return ""
	}
	parts := strings.Split(srcset, ", ")
	for i, p := range parts {
		parts[i] = prefix + p
	}
	return strings.Join(parts, ", ")
}

// pageData is the JSON embedded in each recipe page for the TS runtime.
func pageData(r *recipe.Recipe) (string, error) {
	b, err := json.Marshal(map[string]any{"slug": r.Slug, "title": r.Title, "compiled": r.C})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func writeTemplate(t *template.Template, file string, data any) error {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base", data); err != nil {
		return fmt.Errorf("render %s: %w", file, err)
	}
	return os.WriteFile(file, buf.Bytes(), 0o644)
}

func hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])[:10]
}

const liveReloadJS = `
;(() => { try { const es = new EventSource("/__livereload"); es.onmessage = () => location.reload(); } catch {} })();
`

func writeAssets(customCSS string, hasCustom bool, out string, live bool) (Assets, error) {
	fsys := assets.FS()
	dir := filepath.Join(out, "assets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Assets{}, err
	}
	js, err := fs.ReadFile(fsys, "app.js")
	if err != nil {
		return Assets{}, err
	}
	if live {
		js = append(js, liveReloadJS...)
	}
	css, err := fs.ReadFile(fsys, "app.css")
	if err != nil {
		return Assets{}, err
	}
	if hasCustom {
		extra, err := os.ReadFile(customCSS)
		if err != nil {
			return Assets{}, fmt.Errorf("theme.custom_css: %w", err)
		}
		css = append(append(css, '\n'), extra...)
	}
	a := Assets{JS: "assets/app." + hash(js) + ".js", CSS: "assets/app." + hash(css) + ".css"}
	if err := os.WriteFile(filepath.Join(out, a.JS), js, 0o644); err != nil {
		return a, err
	}
	if err := os.WriteFile(filepath.Join(out, a.CSS), css, 0o644); err != nil {
		return a, err
	}
	return a, nil
}

func writeMeta(out, title, base, absolute string, urls []string, as Assets) error {
	fsys := assets.FS()
	// Service worker: must live at the base so its scope covers the site.
	sw, err := fs.ReadFile(fsys, "sw.js")
	if err != nil {
		return err
	}
	version := hash([]byte(as.JS + as.CSS + time.Now().UTC().Format(time.RFC3339)))
	sw = bytes.ReplaceAll(sw, []byte("__GC_VERSION__"), []byte(version))
	sw = bytes.ReplaceAll(sw, []byte("__GC_BASE__"), []byte(base))
	sw = bytes.ReplaceAll(sw, []byte("__GC_PRECACHE__"), []byte(base+as.JS+`","`+base+as.CSS+`","`+base))
	if err := os.WriteFile(filepath.Join(out, "sw.js"), sw, 0o644); err != nil {
		return err
	}
	manifest, _ := json.MarshalIndent(map[string]any{
		"name": title, "short_name": title, "start_url": base, "scope": base, "display": "standalone",
		"background_color": "#fafaf9", "theme_color": "#ea580c",
		"icons": []map[string]string{{"src": base + "icon.svg", "sizes": "any", "type": "image/svg+xml", "purpose": "any maskable"}},
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "manifest.webmanifest"), manifest, 0o644); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(out, "icon.svg")); errors.Is(err, os.ErrNotExist) {
		icon, err := fs.ReadFile(fsys, "icon.svg")
		if err == nil {
			if err := os.WriteFile(filepath.Join(out, "icon.svg"), icon, 0o644); err != nil {
				return err
			}
		}
	}
	robots := "User-agent: *\nAllow: /\n"
	if absolute != "" {
		robots += "Sitemap: " + absolute + base + "sitemap.xml\n"
		var sb strings.Builder
		sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
		for _, u := range urls {
			fmt.Fprintf(&sb, "  <url><loc>%s</loc></url>\n", esc(absolute+u))
		}
		sb.WriteString("</urlset>\n")
		if err := os.WriteFile(filepath.Join(out, "sitemap.xml"), []byte(sb.String()), 0o644); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(out, "robots.txt")); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(filepath.Join(out, "robots.txt"), []byte(robots), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	o, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(o, in); err != nil {
		_ = o.Close()
		return err
	}
	return o.Close()
}

func copyDir(src, dst string) error {
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return copyFile(p, filepath.Join(dst, rel))
	})
}
