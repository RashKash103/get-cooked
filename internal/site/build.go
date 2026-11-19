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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/RashKash103/get-cooked/internal/assets"
	"github.com/RashKash103/get-cooked/internal/config"
	"github.com/RashKash103/get-cooked/internal/i18n"
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
	// Version of get-cooked shown in the footer ("0.6.0"; "dev" or empty
	// for local builds).
	Version string
}

// releaseRe matches a released version; dev builds and `go install`
// pseudo-versions (0.6.1-0.2026…-abcdef) have no release page.
var releaseRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// generator is the get-cooked version shown in the footer, with a link to its
// release notes (or the list of releases when there's no release page).
type generator struct {
	Version, URL string
}

func newGenerator(v string) generator {
	const repo = "https://github.com/RashKash103/get-cooked"
	v = strings.TrimPrefix(v, "v")
	switch {
	case v == "":
		return generator{Version: "dev", URL: repo + "/releases"}
	case releaseRe.MatchString(v):
		return generator{Version: "v" + v, URL: repo + "/releases/tag/v" + v}
	}
	return generator{Version: v, URL: repo + "/releases"}
}

// Assets are the fingerprinted asset paths (relative to the base).
type Assets struct {
	JS  string
	CSS string
}

type page struct {
	Site        *config.Config
	Base        string // links (includes the language prefix)
	Root        string // site root (assets, icons, service worker)
	Lang        string
	Dir         string
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
	Alternates  []alternate
	I18n        template.JS
	ListData    template.JS
	Feed        string
	Generator   generator
	cat         *i18n.Catalog
}

type alternate struct {
	Code, Name, URL string
	Current         bool
}

// T translates a UI key: {{.T "key" "name" "value"}}.
func (p page) T(key string, pairs ...string) string { return p.cat.T(key, pairs...) }

type recipeView struct {
	R             *recipe.Recipe
	M             *runtime.Model
	Image         string
	SrcSet        string
	ImageAlt      string
	Times         template.HTML
	Equipment     template.HTML
	Ingredients   template.HTML
	Steps         template.HTML
	Warnings      template.HTML
	Tips          template.HTML
	Data          template.JS
	NotTranslated bool
	SourceLang    string
	// The default image (index, previews) when a variant is shown.
	DefaultImage, DefaultSrcSet, DefaultAlt string
	HasVariants                             bool
}

type card struct {
	Title, Slug, URL, Image, SrcSet, Category, Time, Search, Initial string
}

type indexView struct {
	Cards      []card
	Categories []string
}

// jsKeys are the UI strings the browser needs.
var jsKeys = []string{
	"back", "all_recipes", "text_size", "timer_pick", "timer_start", "swap", "swap_ingredient", "timers_more", "timers_less", "timer_go_to_step", "list_share", "list_link_copied", "list_import", "list_imported", "convert", "by_weight", "by_volume", "close", "scale_to", "scale_amount", "scale_go", "progress_cleared", "in_progress", "notes", "ingredients", "equipment", "steps", "source", "units", "units_us", "units_metric", "units_original", "uses",
	"list", "list_add", "list_update", "list_added", "list_remove", "list_make", "list_confirm_clear", "list_items",
	"scale_prompt", "step_of", "all_done", "save", "saved", "copied", "exit_cook", "cook",
	"step", "uses", "unsupported", "cook_on", "cook_on_unsupported", "copied", "copy_failed", "link_copied",
	"timer_started", "timer_set", "timer_done", "timer_done_short", "timer_add", "timer_pause", "timer_resume",
	"timer_cancel", "timer_dismiss", "timer_clock_app", "mark_done", "theme", "theme_system", "theme_light", "theme_dark",
}

type builder struct {
	s                       *Site
	opts                    BuildOptions
	out, root, absolute     string
	as                      Assets
	recipeTpl, indexTpl, nf *template.Template
	listTpl, offlineTpl     *template.Template
	imgs                    *imageProcessor
	sitemap                 []string
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
	b := &builder{s: s, opts: opts, out: opts.Out, root: baseURL, imgs: newImageProcessor(cfg.Images.Widths)}
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		b.root = u.Path
		if b.root == "" {
			b.root = "/"
		}
		b.absolute = strings.TrimSuffix(strings.TrimSuffix(baseURL, "/"), strings.TrimSuffix(b.root, "/"))
	}
	if err := os.MkdirAll(b.out, 0o755); err != nil {
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
	if b.recipeTpl, err = parse("recipe.html"); err != nil {
		return err
	}
	if b.indexTpl, err = parse("index.html"); err != nil {
		return err
	}
	if b.nf, err = parse("404.html"); err != nil {
		return err
	}
	if b.offlineTpl, err = parse("offline.html"); err != nil {
		return err
	}
	if b.listTpl, err = parse("list.html"); err != nil {
		return err
	}
	if b.as, err = writeAssets(cfg.Path(cfg.Theme.CustomCSS), cfg.Theme.CustomCSS != "", b.out, opts.LiveReload); err != nil {
		return err
	}
	if err := copyDir(cfg.Path(cfg.Static), b.out); err != nil {
		return err
	}
	for _, l := range s.Langs {
		if err := b.lang(l); err != nil {
			return err
		}
	}
	if err := b.writeIcons(); err != nil {
		return err
	}
	if err := b.writeManifest(); err != nil {
		return err
	}
	if err := b.writeOffline(); err != nil {
		return err
	}
	return writeMeta(b.out, b.root, b.absolute, b.sitemap, b.as)
}

func (b *builder) abs(p string) string {
	if b.absolute == "" {
		return ""
	}
	return b.absolute + p
}

func (b *builder) alternates(cur *Lang, path string) []alternate {
	if len(b.s.Langs) < 2 {
		return nil
	}
	src := b.s.Langs[0].Code
	var out []alternate
	for _, l := range b.s.Langs {
		out = append(out, alternate{Code: l.Code, Name: l.Catalog.Name, URL: b.root + l.Prefix(src) + path, Current: l == cur})
	}
	return out
}

func (b *builder) newPage(l *Lang, base, title, desc, canonical, path string) page {
	cat := l.Catalog
	js := map[string]string{}
	for _, k := range jsKeys {
		js[k] = cat.T(k)
	}
	js["_digits"] = cat.Digits
	js["_fracs"] = cat.FracDisplay
	j, _ := json.Marshal(js)
	dir := cat.Dir
	if dir == "" {
		dir = "ltr"
	}
	feed := ""
	if b.absolute != "" {
		feed = base + "feed.xml"
	}
	return page{
		Feed: feed,
		Site: b.s.Config, Base: base, Root: b.root, Lang: l.Code, Dir: dir, Title: title, Description: desc,
		Accent: template.CSS(b.s.Config.Theme.Accent), Assets: b.as, Canonical: canonical,
		Alternates: b.alternates(l, path), I18n: template.JS(j), cat: cat,
		Generator: newGenerator(b.opts.Version),
	}
}

func (b *builder) lang(l *Lang) error {
	cfg := b.s.Config
	src := b.s.Langs[0].Code
	prefix := l.Prefix(src)
	base := b.root + prefix
	out := filepath.Join(b.out, filepath.FromSlash(prefix))
	idx := &indexView{}
	cats := map[string]bool{}
	fr := func(h string) string { return h }
	if l.Catalog.Diagonal() {
		fr = diagonalFractions
	}
	for _, r := range l.Recipes {
		dir := filepath.Join(out, "recipes", r.Slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		pageURL := base + "recipes/" + r.Slug + "/"
		for ii := range r.C.Images {
			im := &r.C.Images[ii]
			if im.Src == "" || isRemote(im.Src) || strings.HasPrefix(im.Src, pageURL) {
				continue
			}
			img, err := b.imgs.process(im.Src, filepath.Dir(r.Path), dir)
			if err != nil {
				return fmt.Errorf("%s: image variant: %w", r.Path, err)
			}
			im.Src, im.SrcSet = pageURL+img.src, prefixSrcSet(pageURL, img.srcset)
		}
		m, err := runtime.Render(r.C, runtime.State{})
		if err != nil {
			return fmt.Errorf("%s: %w", r.Path, err)
		}
		v := &recipeView{
			R: r, M: m, ImageAlt: r.ImageAlt,
			Times:         template.HTML(fr(renderTimes(m))),
			Equipment:     template.HTML(fr(renderEquipment(m))),
			Ingredients:   template.HTML(fr(renderIngredients(m, base, l.Catalog))),
			Steps:         template.HTML(fr(renderSteps(m, base, l.Catalog))),
			Warnings:      template.HTML(fr(renderNotes(m, true))),
			Tips:          template.HTML(fr(renderNotes(m, false))),
			NotTranslated: !l.Translated[r.Slug],
			SourceLang:    src,
		}
		if v.ImageAlt == "" {
			v.ImageAlt = r.Title
		}
		if r.Image != "" {
			img, err := b.imgs.process(r.Image, filepath.Dir(r.Path), dir)
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
		for _, sec := range r.C.Steps {
			for _, st := range sec.Items {
				if st.Image != "" && !isRemote(st.Image) {
					if err := copyFile(filepath.Join(filepath.Dir(r.Path), st.Image), filepath.Join(dir, filepath.Base(st.Image))); err != nil {
						return fmt.Errorf("%s: step image: %w", r.Path, err)
					}
				}
			}
		}
		v.DefaultImage, v.DefaultSrcSet, v.DefaultAlt = v.Image, v.SrcSet, v.ImageAlt
		v.HasVariants = len(r.C.Images) > 0
		if m.Image != nil {
			v.Image, v.SrcSet = m.Image.Src, m.Image.SrcSet
			if m.Image.Alt != "" {
				v.ImageAlt = m.Image.Alt
			}
		}
		data, err := pageData(r)
		if err != nil {
			return err
		}
		v.Data = template.JS(data)
		p := b.newPage(l, base, r.Title+" · "+cfg.Title, r.Description, b.abs(pageURL), "recipes/"+r.Slug+"/")
		p.Recipe = v
		p.BodyClass = "page-recipe"
		if v.DefaultImage != "" {
			p.OGImage = v.DefaultImage
			if !isRemote(v.DefaultImage) {
				p.OGImage = b.abs(v.DefaultImage)
			}
		}
		p.JSONLD = template.JS(jsonLD(r, m, p.OGImage, b.abs(pageURL), l.Code))
		if err := writeTemplate(b.recipeTpl, filepath.Join(dir, "index.html"), p); err != nil {
			return err
		}
		b.sitemap = append(b.sitemap, pageURL)

		search := []string{r.Title, r.Category, r.Cuisine, r.Description}
		search = append(search, r.Tags...)
		for _, s := range m.Ingredients {
			for _, it := range s.Items {
				search = append(search, it.Name)
			}
		}
		c := card{
			Title: r.Title, Slug: r.Slug, URL: pageURL, Category: r.Category, Image: v.DefaultImage, SrcSet: v.DefaultSrcSet,
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
	ip := b.newPage(l, base, cfg.Title, cfg.Description, b.abs(base), "")
	ip.Index = idx
	ip.BodyClass = "page-index"
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := writeTemplate(b.indexTpl, filepath.Join(out, "index.html"), ip); err != nil {
		return err
	}
	b.sitemap = append(b.sitemap, base)
	if b.absolute != "" {
		if err := writeFeed(filepath.Join(out, "feed.xml"), b.absolute, base, cfg.Title, l); err != nil {
			return err
		}
	}
	// Shopping list: rendered in the browser from local storage.
	system := cfg.Units.DefaultSystem
	if l.Catalog.System != "" {
		system = l.Catalog.System
	}
	ld, err := json.Marshal(map[string]any{"units": l.Units, "system": system, "categories": l.Catalog.Categories})
	if err != nil {
		return err
	}
	lp := b.newPage(l, base, l.Catalog.T("list")+" · "+cfg.Title, "", "", "list/")
	lp.ListData = template.JS(ld)
	lp.BodyClass = "page-list"
	if err := os.MkdirAll(filepath.Join(out, "list"), 0o755); err != nil {
		return err
	}
	if err := writeTemplate(b.listTpl, filepath.Join(out, "list", "index.html"), lp); err != nil {
		return err
	}
	np := b.newPage(l, base, l.Catalog.T("not_found")+" · "+cfg.Title, "", "", "")
	np.BodyClass = "page-404"
	return writeTemplate(b.nf, filepath.Join(out, "404.html"), np)
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
	b, err := json.Marshal(map[string]any{"slug": r.Slug, "title": r.Title, "description": r.Description, "source": r.Source, "compiled": r.C})
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

func writeMeta(out, base, absolute string, urls []string, as Assets) error {
	fsys := assets.FS()
	// Service worker: must live at the base so its scope covers the site.
	sw, err := fs.ReadFile(fsys, "sw.js")
	if err != nil {
		return err
	}
	version := hash([]byte(as.JS + as.CSS + time.Now().UTC().Format(time.RFC3339)))
	prelude, _ := json.Marshal(map[string]any{
		"version":  version,
		"base":     base,
		"offline":  base + "offline.html",
		"precache": []string{base + as.JS, base + as.CSS, base, base + "offline.html", base + "icon-192.png"},
	})
	sw = append([]byte("self.__GC="+string(prelude)+";\n"), sw...)
	if err := os.WriteFile(filepath.Join(out, "sw.js"), sw, 0o644); err != nil {
		return err
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

// writeFeed writes an Atom feed of the newest recipes.
func writeFeed(path, absolute, base, title string, l *Lang) error {
	var sb strings.Builder
	updated := "1970-01-01T00:00:00Z"
	n := 0
	var entries strings.Builder
	for _, r := range l.Recipes {
		if n == 20 {
			break
		}
		n++
		date := r.Date
		if date == "" {
			date = "1970-01-01"
		}
		ts := date + "T00:00:00Z"
		if ts > updated {
			updated = ts
		}
		u := absolute + base + "recipes/" + r.Slug + "/"
		fmt.Fprintf(&entries, "  <entry>\n    <title>%s</title>\n    <link href=\"%s\"/>\n    <id>%s</id>\n    <updated>%s</updated>\n    <summary>%s</summary>\n  </entry>\n",
			esc(r.Title), esc(u), esc(u), ts, esc(r.Description))
	}
	fmt.Fprintf(&sb, "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<feed xmlns=\"http://www.w3.org/2005/Atom\" xml:lang=\"%s\">\n  <title>%s</title>\n  <link href=\"%s\"/>\n  <link rel=\"self\" href=\"%sfeed.xml\"/>\n  <id>%s</id>\n  <updated>%s</updated>\n",
		esc(l.Code), esc(title), esc(absolute+base), esc(absolute+base), esc(absolute+base), updated)
	sb.WriteString(entries.String())
	sb.WriteString("</feed>\n")
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// writeOffline writes the page the service worker shows when the reader is
// offline and opens a page that isn't cached yet (source language).
func (b *builder) writeOffline() error {
	l := b.s.Langs[0]
	p := b.newPage(l, b.root, l.Catalog.T("offline")+" · "+b.s.Config.Title, "", "", "")
	p.BodyClass = "page-offline"
	return writeTemplate(b.offlineTpl, filepath.Join(b.out, "offline.html"), p)
}
