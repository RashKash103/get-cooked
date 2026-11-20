package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RashKash103/get-cooked/internal/i18n"
	"github.com/RashKash103/get-cooked/internal/recipe"
)

func TestSourceHTML(t *testing.T) {
	cat := i18n.English()
	const v = "/pdf/?f=b.pdf"
	cases := []struct {
		src    recipe.Source
		viewer string
		want   string
	}{
		{recipe.Source{Name: "Grandma"}, "", "Source: Grandma"},
		{recipe.Source{URL: "https://x.test/a"}, "", `Source: <a href="https://x.test/a" rel="noopener">https://x.test/a</a>`},
		{recipe.Source{Name: "Notebook", Page: 12}, "", "Source: Notebook, p. 12"},
		{recipe.Source{Name: "Book", Credit: "Modified from"}, "", "Modified from Book"},
		{recipe.Source{Name: "Book", Credit: "Adapted from {source}, with less sugar."}, "", "Adapted from Book, with less sugar."},
		{recipe.Source{Name: "Book", PDF: "b.pdf", Page: 3, Credit: "From {source}"}, v,
			`From <a class="source-pdf" href="/pdf/?f=b.pdf" rel="noopener">Book, p. 3</a>`},
		{recipe.Source{PDF: "dir/my-book.pdf"}, v, `Source: <a class="source-pdf" href="/pdf/?f=b.pdf" rel="noopener">my-book</a>`},
		{recipe.Source{Name: "Book", PDF: "b.pdf", Page: 3, Credit: "{source} ({page})"}, v,
			`<a class="source-pdf" href="/pdf/?f=b.pdf" rel="noopener">Book</a> (<a class="source-pdf" href="/pdf/?f=b.pdf" rel="noopener">p. 3</a>)`},
		{recipe.Source{Name: "Site", URL: "https://x.test", PDF: "b.pdf"}, v,
			`Source: <a href="https://x.test" rel="noopener">Site</a>, <a class="source-pdf" href="/pdf/?f=b.pdf" rel="noopener">PDF</a>`},
		{recipe.Source{Name: "<b>"}, "", "Source: &lt;b&gt;"},
	}
	for _, c := range cases {
		if got := string(sourceHTML(cat, &c.src, c.viewer)); got != c.want {
			t.Errorf("%+v:\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}

func TestSourcePDF(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(exampleSite, dir); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dir, LoadOptions{})
	if err != nil || s.Diags.HasErrors() {
		t.Fatal(err, s.Diags)
	}
	out := t.TempDir()
	if err := Build(s, BuildOptions{Out: out}); err != nil {
		t.Fatal(err)
	}
	pdfs, _ := filepath.Glob(filepath.Join(out, "assets", "pdf", "example-cookbook.*.pdf"))
	if len(pdfs) != 1 {
		t.Fatalf("published PDFs: %v", pdfs)
	}
	page, _ := os.ReadFile(filepath.Join(out, "recipes", "chocolate-chip-cookies", "index.html"))
	if !strings.Contains(string(page), `href="/pdf/?f=`+filepath.Base(pdfs[0])+`&amp;p=2&amp;`) {
		t.Error("recipe should link to the PDF viewer at page 2")
	}
	if _, err := os.Stat(filepath.Join(out, "pdf", "index.html")); err != nil {
		t.Error("viewer page missing")
	}
	if lazy, _ := filepath.Glob(filepath.Join(out, "assets", "lazy", "pdfjs-*.js")); len(lazy) == 0 {
		t.Error("pdf.js chunk missing from assets/lazy")
	}

	// A missing PDF is an error.
	_ = os.Remove(filepath.Join(dir, "books", "example-cookbook.pdf"))
	s2, err := Load(dir, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Diags.HasErrors() || !strings.Contains(s2.Diags.Error(), "source.pdf /books/example-cookbook.pdf not found") {
		t.Errorf("want a missing-PDF error, got %v", s2.Diags)
	}
}
