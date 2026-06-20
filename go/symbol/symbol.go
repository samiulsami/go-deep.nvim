package symbol

import (
	"go/parser"
	"go/token"
	"strconv"
)

const nullByte = "\x00"

type Kind int

const (
	TypeKind      Kind = 5
	InterfaceKind Kind = 11
	FunctionKind  Kind = 12
	VariableKind  Kind = 13
	ConstantKind  Kind = 14
	StructKind    Kind = 23
)

func (k Kind) String() string {
	switch k {
	case TypeKind:
		return "t"
	case InterfaceKind:
		return "i"
	case FunctionKind:
		return "f"
	case VariableKind:
		return "v"
	case ConstantKind:
		return "c"
	case StructKind:
		return "s"
	default:
		return ""
	}
}

func SupportedKind(kind Kind) bool {
	switch kind {
	case TypeKind, InterfaceKind, FunctionKind, VariableKind, ConstantKind, StructKind:
		return true
	default:
		return false
	}
}

type Symbol struct {
	Name        string
	ImportPath  string
	PackageName string
	Kind        Kind
	IsLocal     bool
	IsVendored  bool
	Location    Location
	Haystack    string
}

type Location struct {
	Path  string
	Range Range
}

type Range struct {
	Start Position
	End   Position
}

type Position struct {
	Line      int
	Character int
}

func BuildHaystack(sym *Symbol) string {
	if sym == nil {
		return ""
	}
	pkg := sym.PackageName
	if pkg == "" && sym.Location.Path != "" {
		pkg = ParsePackageNameFromFile(sym.Location.Path)
		sym.PackageName = pkg
	}
	if pkg == "" {
		return sym.Name
	}
	return pkg + nullByte + sym.Name
}

func ParsePackageNameFromFile(path string) string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
	if err != nil || file.Name == nil {
		return ""
	}
	return file.Name.Name
}

func Hash(sym *Symbol) string {
	if sym == nil {
		return ""
	}
	return sym.Name + "#" + strconv.Itoa(int(sym.Kind)) + "#" + sym.ImportPath
}
