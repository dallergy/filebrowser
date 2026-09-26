//go:build unix

package fbhttp

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/users"
)

// serveWithin runs the request and fails the test, instead of hanging it, if
// the handler has not answered in time.
func serveWithin(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(rec, req)
	}()
	select {
	case <-done:
		return rec
	case <-time.After(5 * time.Second):
		t.Fatalf("VULNERABLE: %s %s hung on a named pipe", req.Method, req.URL)
		return nil
	}
}

// Regression for GHSA-8q5j-8wcr-8v2v: only the single-file raw handler skipped
// named pipes. Archiving a directory that holds one, or downloading one through
// a public share, opened the pipe and blocked forever with no writer.
func TestDownloadsDoNotOpenNamedPipes(t *testing.T) {
	userScope := t.TempDir()
	dir := filepath.Join(userScope, "shared", "f")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "regular.txt"), []byte("regular"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o644); err != nil {
		t.Skipf("cannot create named pipe: %v", err)
	}
	// A symlink to the pipe: file.Mode describes the link, not the pipe.
	if err := os.Symlink("pipe", filepath.Join(dir, "pipe-link")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	key := []byte("test-signing-key")
	perm := users.Permissions{Download: true, Share: true}
	st := scopedUserStorage(t, userScope, perm, key)
	signed := signToken(t, perm, key)
	for _, p := range []string{"/shared/f/pipe", "/shared/f"} {
		if err := st.Share.Save(&share.Link{Hash: filepath.Base(p), UserID: 1, Path: p}); err != nil {
			t.Fatal(err)
		}
	}

	raw := func(target string) *http.Request {
		req, _ := http.NewRequest(http.MethodGet, target, http.NoBody)
		req.Header.Set("X-Auth", signed)
		return req
	}
	public := func(target, query string) *http.Request {
		return newHTTPRequest(t, func(r *http.Request) {
			r.URL.Path = target
			r.URL.RawQuery = query
		})
	}
	onlyRegular := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body=%q; want 200", rec.Code, rec.Body.String())
		}
		zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
		if err != nil {
			t.Fatalf("failed to read zip: %v", err)
		}
		for _, f := range zr.File {
			if f.Name != "f/regular.txt" && f.Name != "regular.txt" {
				t.Errorf("unexpected archive entry %q", f.Name)
			}
		}
	}

	t.Run("directory archive", func(t *testing.T) {
		onlyRegular(t, serveWithin(t, handle(rawHandler, "", st, &settings.Server{}), raw("/shared/f/?algo=zip")))
	})

	t.Run("symlink to pipe", func(t *testing.T) {
		serveWithin(t, handle(rawHandler, "", st, &settings.Server{}), raw("/shared/f/pipe-link"))
	})

	t.Run("public share of a pipe", func(t *testing.T) {
		serveWithin(t, handle(publicDlHandler, "", st, &settings.Server{}), public("pipe", ""))
	})

	t.Run("public share of a directory holding a pipe", func(t *testing.T) {
		onlyRegular(t, serveWithin(t, handle(publicDlHandler, "", st, &settings.Server{}), public("f/", "algo=zip")))
	})
}
