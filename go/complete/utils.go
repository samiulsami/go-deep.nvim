package complete

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

func buildItems(req Request, syms []*symbol.Symbol) []CompletionItem {
	items := make([]CompletionItem, len(syms))
	packageNames := make(map[string]string)
	for i, s := range syms {
		alias := resolvePackageAlias(s, req.ImportedPaths, packageNames)
		items[i] = buildCompletionItem(s, alias)
	}
	return items
}

func buildCompletionItem(s *symbol.Symbol, alias string) CompletionItem {
	label := alias + "." + s.Name
	detail := `"` + s.ImportPath + `"`
	kindStr := s.Kind.String()
	if kindStr == "" {
		kindStr = " "
	}

	ud := userDataWrap{
		GoDeep: userData{
			ImportPath:   s.ImportPath,
			PackageAlias: alias,
			Path:         s.Location.Path,
			Range: userDataRange{
				Start: userDataPosition{Line: s.Location.Range.Start.Line, Character: s.Location.Range.Start.Character},
				End:   userDataPosition{Line: s.Location.Range.End.Line, Character: s.Location.Range.End.Character},
			},
		},
	}
	udJSON, err := json.Marshal(ud)
	if err != nil {
		log.Printf("completion item user_data marshal: %v", err)
		udJSON = []byte("{}")
	}

	return CompletionItem{
		Word:     label,
		Abbr:     label,
		Menu:     detail,
		Info:     parseDocumentationBeforeLine(s.Location.Path, s.Location.Range.Start.Line),
		Kind:     kindStr,
		ICase:    1,
		Dup:      0,
		UserData: string(udJSON),
	}
}

func resolvePackageAlias(s *symbol.Symbol, importedPaths map[string]string, packageNames map[string]string) string {
	if importedAlias := importedPaths[s.ImportPath]; importedAlias != "" {
		return importedAlias
	}

	pkgName := resolvePackageName(s, packageNames)
	alias := strings.ReplaceAll(pkgName, "-", "_")
	if !packageNameConflict(alias, importedPaths) {
		return alias
	}

	for i := 1; i <= 100; i++ {
		candidate := fmt.Sprintf("%s%d", alias, i)
		if !packageNameConflict(candidate, importedPaths) {
			return candidate
		}
	}
	return alias + "X"
}

func resolvePackageName(s *symbol.Symbol, packageNames map[string]string) string {
	if s.PackageName != "" && validPackageName(s.PackageName) {
		return s.PackageName
	}
	if name := packageNames[s.Location.Path]; name != "" {
		return name
	}
	if name := symbol.ParsePackageNameFromFile(s.Location.Path); name != "" {
		if validPackageName(name) {
			packageNames[s.Location.Path] = name
			return name
		}
	}
	seg := s.ImportPath
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		seg = seg[i+1:]
	}
	return strings.ReplaceAll(seg, "-", "_")
}

func validPackageName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func canImportInternal(importingFile, targetFile string) bool {
	before, _, ok := strings.Cut(targetFile, "/internal")
	if !ok {
		return false
	}

	importingDir := filepath.Dir(importingFile)
	return strings.HasPrefix(importingDir+"/", before+"/")
}

func packageNameConflict(alias string, importedPaths map[string]string) bool {
	if len(alias) == 1 && (strings.HasPrefix(alias, "_") || strings.HasPrefix(alias, ".")) {
		return false
	}
	for _, v := range importedPaths {
		if v == alias {
			return true
		}
	}
	return false
}

type userDataWrap struct {
	GoDeep userData `json:"go_deep"`
}

type userData struct {
	ImportPath   string        `json:"import_path"`
	PackageAlias string        `json:"package_alias"`
	Path         string        `json:"path"`
	Range        userDataRange `json:"range"`
}

type userDataRange struct {
	Start userDataPosition `json:"start"`
	End   userDataPosition `json:"end"`
}

type userDataPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

func parseDocumentationBeforeLine(path string, startLine int) string {
	lines := readDocumentationLines(path)
	if len(lines) == 0 {
		return ""
	}

	if startLine < 0 || startLine > len(lines) {
		return ""
	}
	if docs := scanLineDocs(lines, startLine-1); docs != "" {
		return docs
	}
	if startLine > 0 {
		return scanLineDocs(lines, startLine-2)
	}
	return ""
}

func readDocumentationLines(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("documentation file close: %v", closeErr)
		}
	}()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Printf("error reading file: %v", err)
	}
	return lines
}

func scanLineDocs(lines []string, i int) string {
	if i < 0 || i >= len(lines) {
		return ""
	}
	var docs []string
	for ; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			break
		}
		comment, ok := strings.CutPrefix(line, "//")
		if !ok {
			break
		}
		docs = append([]string{strings.TrimSpace(comment)}, docs...)
	}
	if len(docs) == 0 {
		return ""
	}
	return "```go\n" + strings.Join(docs, "\n") + "\n```"
}
