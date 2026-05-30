// Package module implements glisp module resolution, download, and dependency management.
package module

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Require is a single dependency entry.
type Require struct {
	Path    string
	Version string
}

// ModFile represents the contents of a glisp.mod file.
type ModFile struct {
	Module   string
	Requires []Require
}

// ModFilePath returns the path to glisp.mod in dir.
func ModFilePath(dir string) string {
	return filepath.Join(dir, "glisp.mod")
}

// ReadModFile reads and parses glisp.mod from dir.
// Returns an empty ModFile (no error) if glisp.mod does not exist.
func ReadModFile(dir string) (*ModFile, error) {
	path := ModFilePath(dir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &ModFile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseModFile(string(data))
}

func parseModFile(src string) (*ModFile, error) {
	mf := &ModFile{}
	scanner := bufio.NewScanner(strings.NewReader(src))
	inRequireBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}

		if inRequireBlock {
			if line == ")" {
				inRequireBlock = false
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				mf.Requires = append(mf.Requires, Require{Path: parts[0], Version: parts[1]})
			}
			continue
		}

		if rest, ok := strings.CutPrefix(line, "module "); ok {
			mf.Module = strings.TrimSpace(rest)
			continue
		}
		if line == "require (" {
			inRequireBlock = true
			continue
		}
		if rest, ok := strings.CutPrefix(line, "require "); ok {
			// inline: require github.com/user/lib v1.0.0
			parts := strings.Fields(rest)
			if len(parts) >= 2 {
				mf.Requires = append(mf.Requires, Require{Path: parts[0], Version: parts[1]})
			}
		}
	}
	return mf, scanner.Err()
}

// WriteModFile writes mf to glisp.mod in dir.
func WriteModFile(dir string, mf *ModFile) error {
	var sb strings.Builder
	if mf.Module != "" {
		sb.WriteString("module " + mf.Module + "\n")
	}
	if len(mf.Requires) > 0 {
		sb.WriteString("\nrequire (\n")
		for _, r := range mf.Requires {
			sb.WriteString("\t" + r.Path + " " + r.Version + "\n")
		}
		sb.WriteString(")\n")
	}
	return os.WriteFile(ModFilePath(dir), []byte(sb.String()), 0644)
}

// InitModFile creates a glisp.mod in dir with the given module path.
// Returns an error if glisp.mod already exists.
func InitModFile(dir, modulePath string) error {
	mfPath := ModFilePath(dir)
	if _, err := os.Stat(mfPath); err == nil {
		return fmt.Errorf("glisp.mod already exists in %s", dir)
	}
	mf := &ModFile{Module: modulePath}
	return WriteModFile(dir, mf)
}

// AddRequire adds or updates a require entry in mf.
func (mf *ModFile) AddRequire(path, version string) {
	for i, r := range mf.Requires {
		if r.Path == path {
			mf.Requires[i].Version = version
			return
		}
	}
	mf.Requires = append(mf.Requires, Require{Path: path, Version: version})
}
