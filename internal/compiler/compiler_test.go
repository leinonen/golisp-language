package compiler

import (
	"strings"
	"testing"
)

func TestBuildError(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "missing go.mod",
			output: "go: go.mod file not found in current directory or any parent directory; see 'go help modules'",
			want:   "glisp mod init",
		},
		{
			name:   "missing dependency",
			output: "db.go:5:2: github.com/leinonen/glispdb@v0.1.0: replacement directory /x/y does not exist",
			want:   "glisp get",
		},
		{
			name:   "runtime helper error",
			output: "./glisp_runtime.go:42:1: syntax error",
			want:   "glisp bug",
		},
		{
			name:   "ordinary compile error",
			output: "./main.glsp:3: invalid operation: mismatched types",
			want:   ".glsp source",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := buildError(c.output)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("buildError(%q) = %q, want it to contain %q", c.output, err.Error(), c.want)
			}
		})
	}
}
