package complete

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackageNameConflictBlankImport(t *testing.T) {
	imported := map[string]string{"fmt": "_"}
	if packageNameConflict("_", imported) {
		t.Fatal("blank import alias '_' should never conflict")
	}
}

func TestPackageNameConflictSingleChar(t *testing.T) {
	imported := map[string]string{"fmt": "x"}
	if !packageNameConflict("x", imported) {
		t.Fatal("single-char alias 'x' should conflict if already imported")
	}
}

func TestPackageNameConflictNoConflict(t *testing.T) {
	imported := map[string]string{"fmt": "fmt"}
	if packageNameConflict("log", imported) {
		t.Fatal("'log' should not conflict with imported 'fmt'")
	}
}

func TestPackageNameConflictMatchesExisting(t *testing.T) {
	imported := map[string]string{"fmt": "fmt"}
	if !packageNameConflict("fmt", imported) {
		t.Fatal("'fmt' should conflict with existing alias 'fmt'")
	}
}

func TestValidPackageName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"fmt", true},
		{"my_pkg", true},
		{"pkg2", true},
		{"", false},
		{"2pkg", false},
		{"my-pkg", false},
		{"my.pkg", false},
	}
	for _, c := range cases {
		if got := validPackageName(c.name); got != c.want {
			t.Errorf("validPackageName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestParseDocumentationBeforeLineAdjacentComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	content := "// Foo does something cool.\nfunc Foo() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := parseDocumentationBeforeLine(path, 2)
	if got == "" {
		t.Fatal("expected doc comment, got empty string")
	}
}

func TestParseDocumentationBeforeLineBlankLineBreaks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	content := "// Foo does something.\n\nfunc Foo() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := parseDocumentationBeforeLine(path, 3)
	if got != "" {
		t.Fatalf("expected empty (blank line breaks adjacency), got %q", got)
	}
}

func TestParseDocumentationBeforeLineNoComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	content := "package main\n\nfunc Foo() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := parseDocumentationBeforeLine(path, 3)
	if got != "" {
		t.Fatalf("expected empty (no comment), got %q", got)
	}
}

func TestParseDocumentationBeforeLineFirstLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	content := "func Foo() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := parseDocumentationBeforeLine(path, 1)
	if got != "" {
		t.Fatal("expected empty for first line")
	}
}

func TestParseDocumentationBeforeLineNonexistentFile(t *testing.T) {
	got := parseDocumentationBeforeLine("/nonexistent/file.go", 5)
	if got != "" {
		t.Fatal("expected empty for nonexistent file")
	}
}

func TestParseDocumentationBeforeLineMultiLineComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	content := "// Foo does something.\n// It does it well.\nfunc Foo() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := parseDocumentationBeforeLine(path, 3)
	if got == "" {
		t.Fatal("expected multi-line doc comment")
	}
}
