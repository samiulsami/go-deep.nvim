package symbol

import (
	"fmt"
	"strings"
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
	seg := sym.ImportPath
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		seg = seg[i+1:]
	}
	seg = strings.ReplaceAll(seg, "-", "_")
	if seg == "" {
		return sym.Name
	}
	return seg + nullByte + sym.Name
}

func Hash(sym *Symbol) string {
	if sym == nil {
		return ""
	}
	return fmt.Sprintf("%s#%d#%s", sym.Name, sym.Kind, sym.ImportPath)
}
