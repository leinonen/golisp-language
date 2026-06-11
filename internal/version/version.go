// Package version reports the glisp build version.
package version

import (
	"runtime"
	"runtime/debug"
)

// Version is the release version, injected at build time via
//
//	-ldflags "-X golisp/internal/version.Version=v1.2.3"
//
// (see .github/workflows/release.yml). It is empty in plain `go build` /
// `go install` builds, in which case String() derives a value from the module
// build info instead.
var Version = ""

// String returns the build version: the ldflags-injected release tag when set,
// otherwise the module version from the build info (set by `go install
// path@version`), otherwise a "dev" string carrying the VCS revision when the
// binary was built from a checkout, else plain "dev".
func String() string {
	if Version != "" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev string
	var modified bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		if modified {
			rev += "-dirty"
		}
		return "dev+" + rev
	}
	return "dev"
}

// Full returns a one-line version banner including the Go toolchain and target
// platform, e.g. "glisp v1.2.3 (go1.25.5, linux/amd64)".
func Full() string {
	return "glisp " + String() + " (" + runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH + ")"
}
