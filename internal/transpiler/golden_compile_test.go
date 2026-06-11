package transpiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// importDefectSignals are substrings in `go vet` output that indicate an
// import-emission defect in the transpiler — a missing import (§2.3, the
// "context.Background() with no import" bug), a spurious/unused import (§2.1,
// the "pgx" bare-import bug), or a bogus stdlib import (§3.3, "filepath is not
// in std"). These are emission correctness bugs the transpiler must never
// produce, distinct from the documented `any`-seam type mismatches that many
// fragment goldens legitimately exhibit in isolation.
var importDefectSignals = []string{
	"undefined:",            // missing import or undefined symbol
	"imported and not used", // spurious import
	"is not in std",         // bogus single-segment stdlib import
	"cannot find package",   // unresolvable import path
	"could not import",      // unresolvable import path
}

// TestGoldenCompiles type-checks every *.go.golden with `go vet` and fails only
// on import-emission defects (see importDefectSignals), so the import bug
// cluster from docs/go-interop-exploration.md can't regress behind text-only
// golden comparison. Type mismatches from the `any` seam are expected in
// fragment goldens and are ignored. Skipped under -short (shells out per golden).
func TestGoldenCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping golden compile check in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Skip("no testdata directory")
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go.golden") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".go.golden")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, err := os.ReadFile(filepath.Join("testdata", entry.Name()))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}

			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "prog.go"), src, 0644); err != nil {
				t.Fatalf("write prog.go: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module goldentest\n\ngo 1.21\n"), 0644); err != nil {
				t.Fatalf("write go.mod: %v", err)
			}

			cmd := exec.Command("go", "vet", "./...")
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err == nil {
				return
			}
			for _, sig := range importDefectSignals {
				if strings.Contains(string(out), sig) {
					t.Errorf("generated Go has an import-emission defect (%q):\n%s", sig, out)
					return
				}
			}
		})
	}
}
