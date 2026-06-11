package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoRequireParsing(t *testing.T) {
	content := "module github.com/leinonen/pgxdb\n\ngo-require (\n\tgithub.com/jackc/pgx/v5 v5.7.2\n\tgithub.com/google/uuid v1.6.0\n)\n\nrequire (\n\tgithub.com/user/mathlib v1.0.0\n)\n"
	dir := t.TempDir()
	os.WriteFile(dir+"/glisp.mod", []byte(content), 0644)

	mf, err := ReadModFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if mf.Module != "github.com/leinonen/pgxdb" {
		t.Errorf("module: got %q", mf.Module)
	}
	if len(mf.GoRequires) != 2 {
		t.Errorf("go-require count: got %d, want 2", len(mf.GoRequires))
	}
	if mf.GoRequires[0].Path != "github.com/jackc/pgx/v5" || mf.GoRequires[0].Version != "v5.7.2" {
		t.Errorf("first go-require: %+v", mf.GoRequires[0])
	}
	if len(mf.Requires) != 1 || mf.Requires[0].Path != "github.com/user/mathlib" {
		t.Errorf("requires: %v", mf.Requires)
	}

	// Roundtrip: write then re-read
	out := t.TempDir()
	if err := WriteModFile(out, mf); err != nil {
		t.Fatal(err)
	}
	mf2, _ := ReadModFile(out)
	if len(mf2.GoRequires) != 2 {
		t.Errorf("roundtrip go-require count: got %d", len(mf2.GoRequires))
	}
}

// TestEnsureProjectGoMod covers deriving go.mod from glisp.mod: it creates a
// go.mod with the declared module path when none exists, syncs go-require
// entries into it, and is a no-op when glisp.mod is absent.
func TestEnsureProjectGoMod(t *testing.T) {
	t.Run("absent glisp.mod is a no-op", func(t *testing.T) {
		dir := t.TempDir()
		if err := EnsureProjectGoMod(dir); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); !os.IsNotExist(err) {
			t.Errorf("go.mod should not be created without glisp.mod")
		}
	})

	t.Run("creates go.mod from module path", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "glisp.mod"), []byte("module example.com/myapp\n"), 0644)
		if err := EnsureProjectGoMod(dir); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err != nil {
			t.Fatalf("go.mod not created: %v", err)
		}
		if got := string(data); !contains(got, "module example.com/myapp") {
			t.Errorf("go.mod module path: %q", got)
		}
	})

	t.Run("falls back to dir basename when module line absent", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "glisp.mod"), []byte("\n"), 0644)
		if err := EnsureProjectGoMod(dir); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
		if got := string(data); !contains(got, "module "+filepath.Base(dir)) {
			t.Errorf("expected basename fallback, got %q", got)
		}
	})

	t.Run("syncs go-require into existing go.mod", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "glisp.mod"),
			[]byte("module example.com/app\n\ngo-require (\n\tgithub.com/google/uuid v1.6.0\n)\n"), 0644)
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0644)
		if err := EnsureProjectGoMod(dir); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
		if got := string(data); !contains(got, "github.com/google/uuid v1.6.0") {
			t.Errorf("go-require not synced into go.mod: %q", got)
		}
	})
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// TestProjectReplaceValid covers the self-heal predicate that decides whether a
// project's go.mod replace for a cached module needs rewriting: an absolute
// replace to an existing dir is valid; one to a missing dir (e.g. another
// machine's cache path after a fresh clone) or a missing replace is not.
func TestProjectReplaceValid(t *testing.T) {
	dir := t.TempDir()
	existing := t.TempDir()
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	gomod := "module example.com/proj\n\ngo 1.21\n\n" +
		"require example.com/good v1.0.0\n" +
		"require example.com/bad v1.0.0\n\n" +
		"replace example.com/good v1.0.0 => " + existing + "\n" +
		"replace example.com/bad v1.0.0 => " + missing + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644); err != nil {
		t.Fatal(err)
	}

	if !ProjectReplaceValid(dir, "example.com/good") {
		t.Errorf("replace to existing dir should be valid")
	}
	if ProjectReplaceValid(dir, "example.com/bad") {
		t.Errorf("replace to missing dir should be invalid (needs rewrite)")
	}
	if ProjectReplaceValid(dir, "example.com/absent") {
		t.Errorf("absent replace should be invalid (needs rewrite)")
	}
}
