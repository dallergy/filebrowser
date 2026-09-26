package fbhttp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/asdine/storm/v3"

	"github.com/filebrowser/filebrowser/v2/diskcache"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/storage"
	"github.com/filebrowser/filebrowser/v2/storage/bolt"
	"github.com/filebrowser/filebrowser/v2/users"
)

// shareCleanupFixture has two users, "editor" and "other", scoped to their own
// directories under root, each sharing a file stored as "/x.txt" in their own
// scope, plus an admin scoped to root itself.
type shareCleanupFixture struct {
	root  string
	st    *storage.Storage
	key   []byte
	admin *users.User
}

func newShareCleanupFixture(t *testing.T) *shareCleanupFixture {
	t.Helper()

	root := t.TempDir()
	for _, name := range []string{"editor", "other"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, file := range []string{"x.txt", "y.txt"} {
			if err := os.WriteFile(filepath.Join(root, name, file), []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	db, err := storm.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st, err := bolt.NewStorage(db)
	if err != nil {
		t.Fatalf("failed to get storage: %v", err)
	}

	perm := users.Permissions{Share: true, Download: true, Delete: true, Rename: true, Modify: true, Create: true}
	f := &shareCleanupFixture{root: root, st: st, key: []byte("test-signing-key")}
	for _, u := range []*users.User{
		{Username: "editor", Password: "pw", Scope: "/editor", Perm: perm},
		{Username: "other", Password: "pw", Scope: "/other", Perm: perm},
	} {
		if err := st.Users.Save(u); err != nil {
			t.Fatalf("failed to save %s: %v", u.Username, err)
		}
		for _, file := range []string{"x", "y"} {
			if err := st.Share.Save(&share.Link{Hash: u.Username + "-" + file, UserID: u.ID, Path: "/" + file + ".txt"}); err != nil {
				t.Fatalf("failed to save share: %v", err)
			}
		}
	}

	adminPerm := perm
	adminPerm.Admin = true
	f.admin = &users.User{Username: "admin", Password: "pw", Scope: "/", Perm: adminPerm}
	if err := st.Users.Save(f.admin); err != nil {
		t.Fatalf("failed to save admin: %v", err)
	}
	if err := st.Settings.Save(&settings.Settings{Key: f.key}); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	return f
}

func (f *shareCleanupFixture) serve(t *testing.T, fn handleFunc, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest(method, target, http.NoBody)
	req.Header.Set("X-Auth", signShareTestToken(t, f.admin.ID, f.admin.Username, f.admin.Perm, f.key))
	rec := httptest.NewRecorder()
	handle(fn, "", f.st, &settings.Server{Root: f.root}).ServeHTTP(rec, req)
	return rec
}

// assertShares checks which of the fixture's share links still exist.
func (f *shareCleanupFixture) assertShares(t *testing.T, want map[string]bool) {
	t.Helper()
	for hash, exists := range want {
		_, err := f.st.Share.GetByHash(hash)
		if exists && err != nil {
			t.Errorf("share %s was removed; it points at an unrelated file", hash)
		}
		if !exists && err == nil {
			t.Errorf("VULNERABLE: share %s survived; its file is gone and a new one at that path would be served", hash)
		}
	}
}

// Regression for GHSA-r6pg-pg54-rcr5: deleting a file only removed the shares
// the deleting user owned, so an admin deleting another user's shared file left
// the public link behind, ready to serve whatever is created at that path next.
func TestDeleteRemovesOtherUsersShares(t *testing.T) {
	f := newShareCleanupFixture(t)

	if rec := f.serve(t, resourceDeleteHandler(diskcache.NewNoOp()), http.MethodDelete, "/editor/x.txt"); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d body=%q; want 204", rec.Code, rec.Body.String())
	}

	// other's "/x.txt" is a different file on disk that happens to share the
	// scope-relative path; it must be left alone.
	f.assertShares(t, map[string]bool{"editor-x": false, "editor-y": true, "other-x": true, "other-y": true})

	if rec := f.serve(t, resourceDeleteHandler(diskcache.NewNoOp()), http.MethodDelete, "/other/"); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE dir = %d body=%q; want 204", rec.Code, rec.Body.String())
	}
	f.assertShares(t, map[string]bool{"editor-y": true, "other-x": false, "other-y": false})
}
