package symbol

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildHaystackWithPackageName(t *testing.T) {
	sym := &Symbol{Name: "Println", PackageName: "fmt"}
	got := BuildHaystack(sym)
	if got != "fmt\x00Println" {
		t.Fatalf("expected fmt\\x00Println, got %q", got)
	}
}

func TestBuildHaystackEmptyPackageNameNoPath(t *testing.T) {
	sym := &Symbol{Name: "Println", PackageName: ""}
	got := BuildHaystack(sym)
	if got != "Println" {
		t.Fatalf("expected Println, got %q", got)
	}
}

func TestBuildHaystackNil(t *testing.T) {
	if BuildHaystack(nil) != "" {
		t.Fatal("expected empty string for nil symbol")
	}
}

func TestBuildHaystackResolvesAndMutatesFromFilePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	if err := os.WriteFile(path, []byte("package mypkg\n\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sym := &Symbol{Name: "Foo", PackageName: "", Location: Location{Path: path}}
	got := BuildHaystack(sym)
	if got != "mypkg\x00Foo" {
		t.Fatalf("expected mypkg\\x00Foo, got %q", got)
	}
	if sym.PackageName != "mypkg" {
		t.Fatalf("BuildHaystack should mutate sym.PackageName, got %q", sym.PackageName)
	}
}

func TestParsePackageNameFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	if err := os.WriteFile(path, []byte("package bar\n\nfunc Bar() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ParsePackageNameFromFile(path)
	if got != "bar" {
		t.Fatalf("expected bar, got %q", got)
	}
}

func TestParsePackageNameFromFileNonexistent(t *testing.T) {
	if ParsePackageNameFromFile("/nonexistent/path/file.go") != "" {
		t.Fatal("expected empty string for nonexistent file")
	}
}

func TestHash(t *testing.T) {
	sym := &Symbol{Name: "Println", Kind: FunctionKind, ImportPath: "fmt"}
	got := Hash(sym)
	if got != "Println#12#fmt" {
		t.Fatalf("expected Println#12#fmt, got %q", got)
	}
}

func TestHashNil(t *testing.T) {
	if Hash(nil) != "" {
		t.Fatal("expected empty string for nil symbol")
	}
}

func TestHashDistinguishesByKind(t *testing.T) {
	a := &Symbol{Name: "Foo", Kind: FunctionKind, ImportPath: "fmt"}
	b := &Symbol{Name: "Foo", Kind: TypeKind, ImportPath: "fmt"}
	if Hash(a) == Hash(b) {
		t.Fatal("hashes should differ by kind")
	}
}

func TestHashDistinguishesByImportPath(t *testing.T) {
	a := &Symbol{Name: "Foo", Kind: FunctionKind, ImportPath: "fmt"}
	b := &Symbol{Name: "Foo", Kind: FunctionKind, ImportPath: "log"}
	if Hash(a) == Hash(b) {
		t.Fatal("hashes should differ by import path")
	}
}
