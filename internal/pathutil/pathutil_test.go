package pathutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveLocalNormal(t *testing.T) {
	base := t.TempDir()
	got, err := ResolveLocal(base, "src/main.go")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "src", "main.go")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveLocalNested(t *testing.T) {
	base := t.TempDir()
	got, err := ResolveLocal(base, "a/b/c/d.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "a", "b", "c", "d.txt")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveLocalDot(t *testing.T) {
	base := t.TempDir()
	got, err := ResolveLocal(base, "./file.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "file.txt")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveLocalTraversal(t *testing.T) {
	base := t.TempDir()
	cases := []string{
		"../secret.txt",
		"../../etc/passwd",
		"../../../private/key",
		"sub/../../../escape",
	}
	for _, rel := range cases {
		if _, err := ResolveLocal(base, rel); err == nil {
			t.Errorf("expected traversal rejection for %q", rel)
		}
	}
}

func TestResolveLocalAbsolute(t *testing.T) {
	base := t.TempDir()
	abs := filepath.Join(base, "abs.txt")
	if _, err := ResolveLocal(base, abs); err == nil {
		t.Fatal("expected absolute path rejection")
	}
}

func TestResolveRemote(t *testing.T) {
	cases := []struct {
		base, rel, want string
	}{
		{"/var/www/app", "src/main.go", "/var/www/app/src/main.go"},
		{"/var/www/app", "a/b/c.txt", "/var/www/app/a/b/c.txt"},
		{"/var/www/app", "./file.txt", "/var/www/app/file.txt"},
		{"/var/www/app", `src\file.txt`, "/var/www/app/src/file.txt"},
	}
	for _, tc := range cases {
		got, err := ResolveRemote(tc.base, tc.rel)
		if err != nil {
			t.Errorf("ResolveRemote(%q, %q): %v", tc.base, tc.rel, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ResolveRemote(%q, %q) = %q, want %q", tc.base, tc.rel, got, tc.want)
		}
	}
}

func TestResolveRemoteTraversal(t *testing.T) {
	cases := []string{
		"../secret.txt",
		"../../etc/passwd",
		"../../../private/key",
		"/absolute/path",
	}
	for _, rel := range cases {
		if _, err := ResolveRemote("/var/www/app", rel); err == nil {
			t.Errorf("expected traversal rejection for %q", rel)
		}
	}
}

func TestValidateLocalFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "good.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ValidateLocalFile(dir, "good.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := resolveSymlinks(t, file)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestValidateLocalFileMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := ValidateLocalFile(dir, "missing.txt"); err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestValidateLocalFileDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateLocalFile(dir, "subdir"); err == nil {
		t.Fatal("expected directory rejection")
	}
}

func TestValidateLocalFileSymlinkEscapes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	dir := t.TempDir()
	outside := t.TempDir()
	outFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateLocalFile(dir, "link/secret.txt"); err == nil {
		t.Fatal("expected symlink escape rejection")
	}
}

func TestValidateLocalFileSymlinkInside(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(sub, "file.txt")
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(sub, link); err != nil {
		t.Fatal(err)
	}
	got, err := ValidateLocalFile(dir, "link/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := resolveSymlinks(t, target)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func resolveSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestValidateLocalFileBrokenSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "nonexistent"), link); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateLocalFile(dir, "broken"); err == nil {
		t.Fatal("expected broken symlink rejection")
	}
}

