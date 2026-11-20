package site

import (
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/RashKash103/get-cooked/internal/i18n"
	"github.com/RashKash103/get-cooked/internal/recipe"
)

// pdfDir is where source PDFs are published, under the site root. Files are
// fingerprinted, so the service worker can cache them for offline use.
const pdfDir = "assets/pdf/"

// publishPDF copies a source PDF into the site once (however many recipes
// and languages use it) and returns its published file name.
func (b *builder) publishPDF(src string) (string, error) {
	if name, ok := b.pdfs[src]; ok {
		return name, nil
	}
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, in); err != nil {
		return "", err
	}
	stem := recipe.Slugify(strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)), "-")
	if stem == "" {
		stem = "source"
	}
	name := stem + "." + hex.EncodeToString(h.Sum(nil))[:10] + ".pdf"
	if err := copyFile(src, filepath.Join(b.out, filepath.FromSlash(pdfDir), name)); err != nil {
		return "", err
	}
	b.pdfs[src] = name
	return name, nil
}

// viewerURL opens a published PDF at a page in the built-in viewer.
func viewerURL(base, file string, page int, slug, title string) string {
	q := url.Values{}
	q.Set("f", file)
	if page > 0 {
		q.Set("p", strconv.Itoa(page))
	}
	q.Set("r", slug)
	q.Set("t", title)
	return base + "pdf/?" + q.Encode()
}

// sourceHTML renders the credit line: the credit text (default "Source:")
// with {source} replaced by the linked name and {page} by the page. A PDF
// opens in the viewer; with a web link as well, the page links to the PDF.
func sourceHTML(cat *i18n.Catalog, src *recipe.Source, viewer string) template.HTML {
	if src == nil || src.Label() == "" {
		return ""
	}
	label := esc(src.Label())
	page := ""
	if src.Page > 0 {
		page = esc(cat.T("source_page", "n", cat.Num(strconv.Itoa(src.Page))))
	}
	link := func(href, text, class string) string {
		c := ""
		if class != "" {
			c = ` class="` + class + `"`
		}
		return `<a` + c + ` href="` + esc(href) + `" rel="noopener">` + text + `</a>`
	}
	credit := src.Credit
	if credit == "" {
		credit = cat.T("source") + ": {source}"
	} else if !strings.Contains(credit, "{source}") {
		credit = strings.TrimRight(credit, " ") + " {source}"
	}
	separatePage := strings.Contains(credit, "{page}")
	var name, pg string
	switch {
	case src.URL != "":
		name = link(src.URL, label, "")
		if viewer != "" {
			text := page
			if text == "" {
				text = esc(cat.T("source_pdf"))
			}
			pg = link(viewer, text, "source-pdf")
		} else {
			pg = page
		}
	case viewer != "" && page == "":
		name = link(viewer, label, "source-pdf")
	case viewer != "" && separatePage:
		name, pg = link(viewer, label, "source-pdf"), link(viewer, page, "source-pdf")
	case viewer != "":
		name = link(viewer, label+", "+page, "source-pdf")
	default:
		name, pg = label, page
	}
	out := esc(credit)
	if !separatePage && pg != "" {
		out = strings.Replace(out, "{source}", "{source}, {page}", 1)
	}
	out = strings.Replace(out, "{source}", name, 1)
	if pg == "" {
		out = strings.Replace(strings.Replace(out, ", {page}", "", 1), "{page}", "", 1)
	}
	out = strings.Replace(out, "{page}", pg, 1)
	return template.HTML(out)
}

// sourceJSON is the source as the browser's exports see it: name is the
// display label (name, URL or PDF file name).
func sourceJSON(s *recipe.Source) map[string]any {
	if s == nil || s.Label() == "" {
		return nil
	}
	return map[string]any{"name": s.Label(), "url": s.URL, "page": s.Page, "credit": s.Credit}
}
