package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Verify SPA deep links without turning missing API routes or paths into filesystem access.
func TestSiteRoutesAndBoundaries(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"index.html": "<h1>Relay</h1>", "assets/app-test.js": "const relay = true;", "secret.txt": "private"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	site, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := site.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, path := range []string{"/", "/events", "/endpoint", "/events/5a9c38c7-e229-4dad-a702-b03780ba69a7"} {
		w := httptest.NewRecorder()
		site.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.String() != "<h1>Relay</h1>" {
			t.Fatalf("deep link failed: %s", path)
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("document protection missing")
		}
	}
	for _, path := range []string{"/v1/unknown", "/secret.txt", "/assets/../secret.txt", "/assets/%2e%2e/secret.txt",
		"/assets/", "/assets/missing.js", "/unknown", "/events/bad-id", "/.env", "/assets/.env"} {
		w := httptest.NewRecorder()
		site.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("unexpected path served: %s", path)
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/assets/app-test.js", nil)
	r.Header.Set("Range", "bytes=0-4")
	site.ServeHTTP(w, r)
	if w.Code != http.StatusPartialContent || w.Body.String() != "const" {
		t.Fatal("bounded asset range failed")
	}
	w = httptest.NewRecorder()
	site.ServeHTTP(w, httptest.NewRequest("POST", "/events", nil))
	if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatal("mutation reached static handler")
	}
	w = httptest.NewRecorder()
	site.ServeHTTP(w, httptest.NewRequest("HEAD", "/", nil))
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("HEAD boundary failed")
	}
}

func TestSiteFailsClosedForMissingBuildAndSymlinkEscape(t *testing.T) {
	directory := t.TempDir()
	if _, err := Open(directory); err == nil {
		t.Fatal("missing build accepted")
	}
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("Relay"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "assets", "leak.js")); err != nil {
		t.Fatal(err)
	}
	site, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := site.Close(); err != nil {
			t.Error(err)
		}
	}()
	w := httptest.NewRecorder()
	site.ServeHTTP(w, httptest.NewRequest("GET", "/assets/leak.js", nil))
	if w.Code != 404 {
		t.Fatal("symlink escaped web root")
	}
}
