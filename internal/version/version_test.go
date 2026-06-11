package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestStringPrefersInjectedVersion(t *testing.T) {
	old := Version
	defer func() { Version = old }()

	Version = "v9.9.9"
	if got := String(); got != "v9.9.9" {
		t.Errorf("String() = %q, want injected %q", got, "v9.9.9")
	}
}

func TestStringFallbackIsNonEmpty(t *testing.T) {
	old := Version
	defer func() { Version = old }()

	Version = ""
	if got := String(); got == "" {
		t.Error("String() should never be empty when no version is injected")
	}
}

func TestFullIncludesPlatform(t *testing.T) {
	old := Version
	defer func() { Version = old }()

	Version = "v1.0.0"
	got := Full()
	for _, want := range []string{"glisp", "v1.0.0", runtime.Version(), runtime.GOOS + "/" + runtime.GOARCH} {
		if !strings.Contains(got, want) {
			t.Errorf("Full() = %q, want it to contain %q", got, want)
		}
	}
}
