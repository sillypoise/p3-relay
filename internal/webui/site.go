// Package webui serves the compiled dashboard from the API's origin, without another web server.
package webui

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/sillypoise/p3-relay/internal/identifier"
)

type Site struct{ root *os.Root }

func Open(directory string) (*Site, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, errors.New("web directory unavailable")
	}
	info, err := root.Stat("index.html")
	if err != nil || info.Mode().IsRegular() == false {
		if closeError := root.Close(); closeError != nil {
			slog.Error("web root close failed")
		}
		return nil, errors.New("web index unavailable")
	}
	return &Site{root: root}, nil
}

func (site *Site) Close() error { return site.root.Close() }

func (site *Site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	filename := siteFilename(r.URL.Path)
	if filename == "" {
		http.NotFound(w, r)
		return
	}
	file, err := site.root.Open(filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Warn("web asset close failed")
		}
	}()
	info, err := file.Stat()
	if err != nil || info.Mode().IsRegular() == false {
		http.NotFound(w, r)
		return
	}
	if filename == "index.html" {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; "+
			"style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; "+
			"base-uri 'none'; form-action 'self'")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeContent(w, r, filename, info.ModTime(), file)
}

func siteFilename(path string) string {
	switch path {
	case "/", "/events", "/endpoint":
		return "index.html"
	}
	if strings.HasPrefix(path, "/events/") {
		if identifier.ValidUUID(strings.TrimPrefix(path, "/events/")) {
			return "index.html"
		}
		return ""
	}
	name := strings.TrimPrefix(path, "/")
	if strings.HasPrefix(name, "assets/") == false || fs.ValidPath(name) == false {
		return ""
	}
	// Vite's generated assets live in a flat directory; no listings or arbitrary files are exposed.
	basename := strings.TrimPrefix(name, "assets/")
	if basename == "" || strings.Contains(basename, "/") || strings.HasPrefix(basename, ".") {
		return ""
	}
	return name
}
