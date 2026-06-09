package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBomFromGoModEmitsPurl(t *testing.T) {
	dir := t.TempDir()
	gomod := "module example.com/foo\n\ngo 1.22\n\nrequire (\n\tgolang.org/x/crypto v0.51.0\n\tgithub.com/gin-gonic/gin v1.9.1\n)\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0600); err != nil {
		t.Fatal(err)
	}
	xml, err := bomFromGoMod(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xml, "<purl>pkg:golang/golang.org/x/crypto@v0.51.0</purl>") {
		t.Errorf("missing expected golang PURL in:\n%s", xml)
	}
	if !strings.Contains(xml, `bom-ref="pkg:golang/github.com/gin-gonic/gin@v1.9.1"`) {
		t.Errorf("missing expected bom-ref in:\n%s", xml)
	}
}

func TestDepPurlEcosystems(t *testing.T) {
	cases := map[dep]string{
		{"golang.org/x/crypto", "v0.51.0", "golang"}: "pkg:golang/golang.org/x/crypto@v0.51.0",
		{"Flask", "2.0.1", "pypi"}:                   "pkg:pypi/flask@2.0.1",
		{"jinja_2", "3.0.0", "pypi"}:                 "pkg:pypi/jinja-2@3.0.0",
		{"@babel/core", "7.0.0", "npm"}:              "pkg:npm/@babel%2Fcore@7.0.0",
		{"lodash", "4.17.21", "npm"}:                 "pkg:npm/lodash@4.17.21",
	}
	for d, want := range cases {
		if got := d.purl(); got != want {
			t.Errorf("purl(%+v) = %q, want %q", d, got, want)
		}
	}
}
