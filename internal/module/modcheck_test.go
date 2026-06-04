package module

import (
	"os"
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
