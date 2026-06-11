package module

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CacheDir returns the root module cache directory (~/.glisp/pkg/mod).
func CacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".glisp", "pkg", "mod")
}

// ModuleCacheDir returns the cache dir for a specific module+version.
func ModuleCacheDir(modulePath, version string) string {
	return filepath.Join(CacheDir(), modulePath+"@"+version)
}

// IsCached returns true if the module has been downloaded and transpiled
// (indicated by presence of go.mod in the cache directory).
func IsCached(modulePath, version string) bool {
	_, err := os.Stat(filepath.Join(ModuleCacheDir(modulePath, version), "go.mod"))
	return err == nil
}

// IsLocalPath reports whether p is a relative local path.
func IsLocalPath(p string) bool {
	return strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../")
}

// Download downloads a module to the cache (or resolves a local path) and
// returns the absolute path to the module directory.
// It does NOT transpile; call compiler.TranspileDir on the returned path.
func Download(projectDir, modulePath, version string) (string, error) {
	if IsLocalPath(modulePath) {
		abs, err := filepath.Abs(filepath.Join(projectDir, modulePath))
		if err != nil {
			return "", fmt.Errorf("resolve local path %s: %w", modulePath, err)
		}
		return abs, nil
	}
	return downloadRemote(modulePath, version)
}

// ResolveModulePath resolves the canonical module path for a local directory.
// Reads glisp.mod if present, otherwise falls back to the directory basename.
func ResolveModulePath(moduleDir, fallback string) string {
	mf, err := ReadModFile(moduleDir)
	if err == nil && mf.Module != "" {
		return mf.Module
	}
	return fallback
}

// EnsureGoMod writes a minimal go.mod to moduleDir if one doesn't exist,
// then adds any go-require entries as require directives.
func EnsureGoMod(moduleDir, modulePath string, goReqs []GoRequire) error {
	goModPath := filepath.Join(moduleDir, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		return nil
	}
	content := "module " + modulePath + "\n\ngo 1.21\n"
	if err := os.WriteFile(goModPath, []byte(content), 0644); err != nil {
		return err
	}
	for _, gr := range goReqs {
		ref := gr.Path + "@" + gr.Version
		cmd := exec.Command("go", "mod", "edit", "-require="+ref)
		cmd.Dir = moduleDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("go mod edit -require=%s: %w\n%s", ref, err, out)
		}
	}
	return nil
}

// EnsureProjectGoMod makes go.mod a derived artifact of glisp.mod for a project
// directory. If glisp.mod is present but go.mod is not, it generates go.mod from
// glisp.mod's module path (falling back to the directory basename when the
// module line is absent), so a glisp.mod + *.glsp checkout is a sufficient,
// buildable project. It then syncs every go-require entry from glisp.mod into
// go.mod, wiring app-level Go dependencies that were previously declared but
// never propagated. A no-op when glisp.mod is absent.
func EnsureProjectGoMod(projectDir string) error {
	if _, err := os.Stat(ModFilePath(projectDir)); os.IsNotExist(err) {
		return nil
	}
	mf, err := ReadModFile(projectDir)
	if err != nil {
		return err
	}

	goModPath := filepath.Join(projectDir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		modulePath := mf.Module
		if modulePath == "" {
			if abs, aerr := filepath.Abs(projectDir); aerr == nil {
				modulePath = filepath.Base(abs)
			}
			if modulePath == "" || modulePath == "." || modulePath == string(filepath.Separator) {
				modulePath = "app"
			}
		}
		content := "module " + modulePath + "\n\ngo 1.21\n"
		if err := os.WriteFile(goModPath, []byte(content), 0644); err != nil {
			return err
		}
	}

	for _, gr := range mf.GoRequires {
		ref := gr.Path + "@" + gr.Version
		cmd := exec.Command("go", "mod", "edit", "-require="+ref)
		cmd.Dir = projectDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("go mod edit -require=%s: %w\n%s", ref, err, out)
		}
	}
	return nil
}

// ProjectReplaceValid reports whether the project's go.mod already maps
// modulePath to an existing local directory via an absolute replace directive.
// A committed replace can carry another machine's absolute cache path (the cache
// lives under each developer's home dir), which is invalid after a fresh clone;
// such a replace — and a missing one — must be rewritten to this machine's cache.
// A relative replace is assumed intentional (e.g. a local fork) and left alone.
func ProjectReplaceValid(projectDir, modulePath string) bool {
	target := projectReplaceTarget(projectDir, modulePath)
	if target == "" {
		return false
	}
	if !filepath.IsAbs(target) {
		return true
	}
	info, err := os.Stat(target)
	return err == nil && info.IsDir()
}

// RequireVersion returns the version the project's go.mod requires for path,
// or "" if there is no such require (or go.mod can't be read). Used to read back
// the concrete version `go get` resolved for a Go dependency.
func RequireVersion(projectDir, path string) string {
	cmd := exec.Command("go", "mod", "edit", "-json")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var gm struct {
		Require []struct {
			Path    string
			Version string
		}
	}
	if err := json.Unmarshal(out, &gm); err != nil {
		return ""
	}
	for _, r := range gm.Require {
		if r.Path == path {
			return r.Version
		}
	}
	return ""
}

// projectReplaceTarget returns the path the project's go.mod maps modulePath to
// via a replace directive, or "" if there is none (or go.mod can't be read).
func projectReplaceTarget(projectDir, modulePath string) string {
	cmd := exec.Command("go", "mod", "edit", "-json")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var gm struct {
		Replace []struct {
			Old struct{ Path string }
			New struct{ Path string }
		}
	}
	if err := json.Unmarshal(out, &gm); err != nil {
		return ""
	}
	for _, r := range gm.Replace {
		if r.Old.Path == modulePath {
			return r.New.Path
		}
	}
	return ""
}

// RegisterInGoMod adds require + replace directives for a module in the project's go.mod.
func RegisterInGoMod(projectDir, modulePath, version, moduleDir string) error {
	absModuleDir, err := filepath.Abs(moduleDir)
	if err != nil {
		return fmt.Errorf("abs path %s: %w", moduleDir, err)
	}

	ref := modulePath + "@" + version
	replaceArg := ref + "=" + absModuleDir

	for _, args := range [][]string{
		{"mod", "edit", "-require=" + ref},
		{"mod", "edit", "-replace=" + replaceArg},
	} {
		cmd := exec.Command("go", args...)
		cmd.Dir = projectDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, out)
		}
	}
	return nil
}

func downloadRemote(modulePath, version string) (string, error) {
	destDir := ModuleCacheDir(modulePath, version)
	if _, err := os.Stat(destDir); err == nil {
		return destDir, nil
	}

	url, err := buildDownloadURL(modulePath, version)
	if err != nil {
		return "", err
	}

	fmt.Fprintf(os.Stderr, "glisp: downloading %s\n", url)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}
	if err := extractTarGz(resp.Body, destDir); err != nil {
		os.RemoveAll(destDir)
		return "", fmt.Errorf("extract %s: %w", modulePath, err)
	}
	return destDir, nil
}

func buildDownloadURL(modulePath, version string) (string, error) {
	parts := strings.SplitN(modulePath, "/", 3)
	if len(parts) < 3 || parts[0] != "github.com" {
		return "", fmt.Errorf("unsupported module host (only github.com supported): %s", modulePath)
	}
	user, repo := parts[1], parts[2]
	return fmt.Sprintf("https://github.com/%s/%s/archive/refs/tags/%s.tar.gz", user, repo, version), nil
}

func extractTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// GitHub archives wrap everything in a top-level dir like "repo-v1.0.0/"
		slashPath := filepath.ToSlash(hdr.Name)
		idx := strings.Index(slashPath, "/")
		if idx < 0 || idx == len(slashPath)-1 {
			continue
		}
		relPath := filepath.FromSlash(slashPath[idx+1:])
		target := filepath.Join(destDir, relPath)

		// Prevent path traversal
		cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDest) {
			return fmt.Errorf("invalid path in archive: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(f, tr)
			f.Close()
			if copyErr != nil {
				return copyErr
			}
		}
	}
	return nil
}
