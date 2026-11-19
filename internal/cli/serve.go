package cli

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/RashKash103/get-cooked/internal/site"
)

func serveCmd() *cobra.Command {
	var dir, addr string
	var drafts, watch bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Build into a temporary directory and serve with live reload",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := os.MkdirTemp("", "get-cooked-serve-*")
			if err != nil {
				return err
			}
			defer func() { _ = os.RemoveAll(out) }()
			hub := &reloadHub{}
			rebuild := func() {
				start := time.Now()
				s, err := site.Load(dir, site.LoadOptions{Drafts: drafts, Sweep: false})
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
					return
				}
				printDiags(cmd.ErrOrStderr(), s.Diags, "text")
				if err := site.Build(s, site.BuildOptions{Out: out, BaseURL: "/", LiveReload: watch, Version: Version}); err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
					return
				}
				fmt.Fprintf(cmd.OutOrStdout(), "built %d recipes in %s\n", len(s.Recipes), time.Since(start).Round(time.Millisecond))
				hub.broadcast()
			}
			rebuild()
			mux := http.NewServeMux()
			mux.HandleFunc("/__livereload", hub.serve)
			files := http.FileServer(http.Dir(out))
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				p := filepath.Join(out, filepath.FromSlash(r.URL.Path))
				if st, err := os.Stat(p); err != nil || (st.IsDir() && !exists(filepath.Join(p, "index.html"))) {
					w.WriteHeader(http.StatusNotFound)
					b, _ := os.ReadFile(filepath.Join(out, "404.html"))
					_, _ = w.Write(b)
					return
				}
				files.ServeHTTP(w, r)
			})
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "serving on http://%s\n", ln.Addr())
			srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			if watch {
				go watchDir(ctx, dir, rebuild)
			}
			go func() {
				<-ctx.Done()
				sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = srv.Shutdown(sctx)
			}()
			if err := srv.Serve(ln); err != http.ErrServerClosed {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "source", "s", ".", "site directory")
	cmd.Flags().StringVarP(&addr, "addr", "a", "localhost:8080", "listen address")
	cmd.Flags().BoolVar(&drafts, "drafts", true, "include draft recipes")
	cmd.Flags().BoolVar(&watch, "watch", true, "rebuild and reload on changes")
	return cmd
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// watchDir polls modification times (portable, no extra dependency).
func watchDir(ctx context.Context, dir string, onChange func()) {
	snapshot := func() string {
		var sb strings.Builder
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // skip unreadable entries while polling
			}
			name := d.Name()
			if d.IsDir() && (name == "public" || name == "node_modules" || strings.HasPrefix(name, ".") && p != dir) {
				return filepath.SkipDir
			}
			if info, err := d.Info(); err == nil && !d.IsDir() {
				fmt.Fprintf(&sb, "%s:%d:%d;", p, info.ModTime().UnixNano(), info.Size())
			}
			return nil
		})
		return sb.String()
	}
	last := snapshot()
	t := time.NewTicker(400 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if s := snapshot(); s != last {
				last = s
				onChange()
			}
		}
	}
}

type reloadHub struct {
	mu   sync.Mutex
	subs []chan struct{}
}

func (h *reloadHub) serve(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs = append(h.subs, ch)
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		for i, c := range h.subs {
			if c == ch {
				h.subs = append(h.subs[:i], h.subs[i+1:]...)
				break
			}
		}
		h.mu.Unlock()
	}()
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			fmt.Fprint(w, "data: reload\n\n")
			fl.Flush()
		}
	}
}

func (h *reloadHub) broadcast() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.subs {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}
