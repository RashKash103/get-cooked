// Package skills downloads and installs the get-cooked Claude Code skills.
//
// Release builds fetch the skills tarball attached to their own GitHub
// release, so the skills always match the binary. Development builds fetch
// the latest skills from the main branch.
package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Repo is the GitHub repository skills are downloaded from.
const Repo = "RashKash103/get-cooked"

// AssetName is the release asset containing the skills.
const AssetName = "get-cooked-skills.tar.gz"

var releaseRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

// Source describes where skills come from.
type Source struct {
	URL   string
	Label string // human description, e.g. "release v1.2.0"
}

// SourceFor returns the download source for a build version. The
// GET_COOKED_SKILLS_URL environment variable overrides it (tests, mirrors).
func SourceFor(version string) Source {
	if u := os.Getenv("GET_COOKED_SKILLS_URL"); u != "" {
		return Source{URL: u, Label: u}
	}
	if releaseRe.MatchString(version) {
		v := "v" + strings.TrimPrefix(version, "v")
		return Source{
			URL:   fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", Repo, v, AssetName),
			Label: "release " + v,
		}
	}
	return Source{
		URL:   fmt.Sprintf("https://codeload.github.com/%s/tar.gz/refs/heads/main", Repo),
		Label: "latest main branch (development build)",
	}
}

// Skill is a downloaded skill: its files keyed by slash-separated path.
type Skill struct {
	Name        string
	Description string
	Files       map[string][]byte
}

// Download fetches a tarball (max 20 MB).
func Download(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "get-cooked")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 20<<20))
}

// Parse reads skills from a .tar.gz. Any entry with a path like
// ".../skills/<name>/<file>" belongs to skill <name>, so both the release
// asset (skills/…) and a repository archive (get-cooked-main/skills/…) work.
func Parse(tgz []byte) ([]*Skill, error) {
	zr, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, fmt.Errorf("not a gzip archive: %w", err)
	}
	tr := tar.NewReader(zr)
	byName := map[string]*Skill{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		parts := strings.Split(path.Clean(h.Name), "/")
		idx := -1
		for i, p := range parts {
			if p == "skills" {
				idx = i
				break
			}
		}
		if idx < 0 || len(parts) < idx+3 {
			continue
		}
		name := parts[idx+1]
		rel := strings.Join(parts[idx+2:], "/")
		if !validName(name) || strings.Contains(rel, "..") {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(tr, 5<<20))
		if err != nil {
			return nil, err
		}
		s := byName[name]
		if s == nil {
			s = &Skill{Name: name, Files: map[string][]byte{}}
			byName[name] = s
		}
		s.Files[rel] = b
	}
	var out []*Skill
	for _, s := range byName {
		md, ok := s.Files["SKILL.md"]
		if !ok {
			continue
		}
		s.Description = description(md)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) == 0 {
		return nil, errors.New("the archive contains no skills")
	}
	return out, nil
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func validName(s string) bool { return nameRe.MatchString(s) }

func description(md []byte) string {
	for _, line := range strings.Split(string(md), "\n") {
		if strings.HasPrefix(line, "description:") {
			d := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			if i := strings.Index(d, ". "); i > 0 {
				d = d[:i+1]
			}
			return d
		}
	}
	return ""
}

// Select picks skills by name (all when names is empty).
func Select(all []*Skill, names []string) ([]*Skill, error) {
	if len(names) == 0 {
		return all, nil
	}
	by := map[string]*Skill{}
	var avail []string
	for _, s := range all {
		by[s.Name] = s
		avail = append(avail, s.Name)
	}
	var out []*Skill
	for _, n := range names {
		s, ok := by[n]
		if !ok {
			return nil, fmt.Errorf("unknown skill %q (available: %s)", n, strings.Join(avail, ", "))
		}
		out = append(out, s)
	}
	return out, nil
}

// Existing returns which of the skills already exist in dir.
func Existing(dir string, list []*Skill) []string {
	var out []string
	for _, s := range list {
		if _, err := os.Stat(filepath.Join(dir, s.Name)); err == nil {
			out = append(out, s.Name)
		}
	}
	return out
}

// Install writes skills into dir/<name>/, replacing any existing copy.
func Install(dir string, list []*Skill) error {
	for _, s := range list {
		target := filepath.Join(dir, s.Name)
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		for rel, b := range s.Files {
			dst := filepath.Join(target, filepath.FromSlash(rel))
			if !strings.HasPrefix(dst, target+string(os.PathSeparator)) {
				return fmt.Errorf("refusing to write outside %s: %s", target, rel)
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(dst, b, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// Pack builds the release asset from a skills directory (used by the release
// workflow via `get-cooked skills pack`).
func Pack(dir string, w io.Writer) error {
	zw := gzip.NewWriter(w)
	tw := tar.NewWriter(zw)
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		h := &tar.Header{Name: "skills/" + filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(b)), ModTime: time.Unix(0, 0)}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		_, err = tw.Write(b)
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return zw.Close()
}
