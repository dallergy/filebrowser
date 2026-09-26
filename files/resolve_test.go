package files

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

func TestResolvePath(t *testing.T) {
	root := t.TempDir()
	scope := filepath.Join(root, "scope")
	for _, dir := range []string{"a", "b", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(scope, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		"a/to-b":     "../b",
		"a/dangling": "../b/missing.txt",
		"a/out":      root,
	}
	for link, dest := range links {
		if err := os.Symlink(dest, filepath.Join(scope, link)); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
	}

	fsys := NewScopedFs(afero.NewOsFs(), scope)
	cases := []struct {
		name, want string
		ok         bool
	}{
		{"/", "/", true},
		{"/a", "/a", true},
		{"/.hidden", "/.hidden", true},
		{"/a/to-b", "/b", true},
		{"/a/to-b/file.txt", "/b/file.txt", true},
		{"/a/to-b/new/deep.txt", "/b/new/deep.txt", true},
		{"/a/dangling", "/b/missing.txt", true},
		{"/a/out", "", false},
		{"/a/out/other.txt", "", false},
		{"/a/out/scope/b", "/b", true},
	}
	for _, tc := range cases {
		got, ok := ResolvePath(fsys, tc.name)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ResolvePath(%q) = %q, %v; want %q, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}

	if _, ok := ResolvePath(afero.NewMemMapFs(), "/a"); ok {
		t.Error("ResolvePath on a filesystem not rooted on disk reported ok")
	}
}
