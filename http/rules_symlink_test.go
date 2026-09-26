package fbhttp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filebrowser/filebrowser/v2/diskcache"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

// Regression for GHSA-7w29-q235-57m9: rules were matched against the path as
// requested while the scoped filesystem followed symlinks, so an in-scope link
// into a denied directory let a user read and overwrite what the rules deny.
func TestRuleDeniesSymlinkAlias(t *testing.T) {
	userScope := t.TempDir()
	for _, dir := range []string{"allowed", "denied"} {
		if err := os.MkdirAll(filepath.Join(userScope, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(userScope, "denied", "secret.txt")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userScope, "allowed", "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Planted out-of-band, per the advisory's preconditions.
	if err := os.Symlink("../denied", filepath.Join(userScope, "allowed", "link")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if err := os.Symlink("ok.txt", filepath.Join(userScope, "allowed", "alias.txt")); err != nil {
		t.Fatal(err)
	}

	key := []byte("test-signing-key")
	perm := users.Permissions{Download: true, Modify: true, Create: true}
	st := denyRuleStorage(t, userScope, "/denied", perm, key)
	signed := signToken(t, perm, key)

	serve := func(fn handleFunc, method, target, body string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("X-Auth", signed)
		rec := httptest.NewRecorder()
		handle(fn, "", st, &settings.Server{}).ServeHTTP(rec, req)
		return rec
	}

	for _, target := range []string{"/denied/secret.txt", "/allowed/link/secret.txt", "/allowed/link/"} {
		if rec := serve(rawHandler, http.MethodGet, target, ""); rec.Code != http.StatusForbidden {
			t.Errorf("VULNERABLE: GET %s = %d body=%q; want 403", target, rec.Code, rec.Body.String())
		}
	}

	if rec := serve(resourcePutHandler, http.MethodPut, "/allowed/link/secret.txt", "PWNED"); rec.Code != http.StatusForbidden {
		t.Errorf("VULNERABLE: PUT through the link = %d; want 403", rec.Code)
	}
	if data, _ := os.ReadFile(secret); string(data) != "SECRET" {
		t.Errorf("VULNERABLE: denied file overwritten with %q", data)
	}

	if rec := serve(resourcePostHandler(diskcache.NewNoOp()), http.MethodPost, "/allowed/link/new.txt", "x"); rec.Code != http.StatusForbidden {
		t.Errorf("VULNERABLE: POST through the link = %d; want 403", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(userScope, "denied", "new.txt")); err == nil {
		t.Error("VULNERABLE: file created in the denied directory")
	}

	// Allowed paths, including a link to an allowed file, keep working.
	for _, target := range []string{"/allowed/ok.txt", "/allowed/alias.txt"} {
		if rec := serve(rawHandler, http.MethodGet, target, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d; want 200", target, rec.Code)
		}
	}
}
