package complete

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/samiulsami/go-deep.nvim/go/gopls"
	"github.com/samiulsami/go-deep.nvim/go/index"
	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

type Provider struct {
	client *gopls.GoplsManager
}

func NewProvider(client *gopls.GoplsManager) (*Provider, error) {
	if client == nil {
		return nil, fmt.Errorf("complete: nil gopls manager")
	}
	return &Provider{client: client}, nil
}

func (p *Provider) ValidPrefix(prefix string) bool {
	if prefix == "" {
		return false
	}
	for _, r := range prefix {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '_' {
			continue
		}
		return false
	}
	return true
}

func (p *Provider) IncludeSymbol(req Request, s *symbol.Symbol) bool {
	if s == nil {
		return false
	}
	if req.Options.ExcludeImported && req.ImportedPaths[s.ImportPath] != "" {
		return false
	}
	if s.Location.Path == req.Filepath || sameDir(s.Location.Path, req.Filepath) {
		return false
	}
	if req.Options.ExcludeTestFiles && strings.HasSuffix(s.Location.Path, "_test.go") {
		return false
	}
	if req.Options.ExcludeVendored && s.IsVendored {
		return false
	}
	if req.Options.ExcludeInternal {
		if index.IsInternalImportPath(s.ImportPath) && (!s.IsLocal || !canImportInternal(req.Filepath, s.Location.Path)) {
			return false
		}
	}
	return true
}

func (p *Provider) BuildItems(req Request, syms []*symbol.Symbol) ([]CompletionItem, error) {
	items := make([]CompletionItem, len(syms))
	packageNames := make(map[string]string)
	documentationLines := make(map[string][]string)
	for i, s := range syms {
		alias := resolvePackageAlias(s, req.ImportedPaths, packageNames)
		items[i] = buildCompletionItem(s, alias, documentationLines)
	}
	return items, nil
}

func (p *Provider) FetchSymbols(ctx context.Context, req Request) ([]*symbol.Symbol, error) {
	rawSymbols, err := p.client.WorkspaceSymbol(ctx, req.CWD, req.Prefix)
	if err != nil {
		return nil, err
	}
	if len(rawSymbols) == 0 {
		return nil, nil
	}
	out := make([]*symbol.Symbol, 0, len(rawSymbols))
	for _, raw := range rawSymbols {
		if n, ok := normalize(raw, req.CWD); ok {
			out = append(out, n)
		}
	}
	return out, nil
}

func normalize(raw *gopls.LspSymbol, cwd string) (*symbol.Symbol, bool) {
	if raw == nil || !symbol.SupportedKind(raw.Kind) {
		return nil, false
	}
	name := raw.Name
	if dot := strings.LastIndex(name, "."); dot >= 0 && dot < len(name)-1 {
		name = name[dot+1:]
	}
	if name == "" || !isExported(name) {
		return nil, false
	}
	uri := raw.Location.URI
	if !strings.HasPrefix(uri, "file://") {
		return nil, false
	}
	path := filepath.Clean(strings.TrimPrefix(uri, "file://"))
	cwd = filepath.Clean(cwd)
	isLocal := false
	if cwd != "." && cwd != "" {
		if rel, err := filepath.Rel(cwd, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			isLocal = true
		}
	}
	isVendored := strings.Contains(path, string(filepath.Separator)+"vendor"+string(filepath.Separator))
	sym := &symbol.Symbol{
		Name:        name,
		ImportPath:  raw.ContainerName,
		PackageName: "", // inferred lazily from filepath
		Kind:        raw.Kind,
		IsLocal:     isLocal && !isVendored,
		IsVendored:  isVendored,
		Location: symbol.Location{
			Path:  path,
			Range: raw.Location.Range,
		},
	}
	sym.Haystack = symbol.BuildHaystack(sym)
	return sym, true
}

func isExported(name string) bool {
	return name != "" && name[0] >= 'A' && name[0] <= 'Z'
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

func sameDir(a string, b string) bool {
	return a != "" && b != "" && filepath.Dir(a) == filepath.Dir(b)
}

func canImportInternal(importingFile, targetFile string) bool {
	before, _, ok := strings.Cut(targetFile, "/internal")
	if !ok {
		return false
	}

	importingDir := filepath.Dir(importingFile)
	return strings.HasPrefix(importingDir+"/", before+"/")
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
	if name, ok := parsePackageNameFromFile(s.Location.Path); ok {
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

func buildCompletionItem(s *symbol.Symbol, alias string, documentationLines map[string][]string) CompletionItem {
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
		Info:     parseDocumentationBeforeLine(s.Location.Path, s.Location.Range.Start.Line, documentationLines),
		Kind:     kindStr,
		ICase:    1,
		Dup:      0,
		UserData: string(udJSON),
	}
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

func parsePackageNameFromFile(path string) (string, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
	if err != nil {
		return "", false
	}
	if file.Name == nil || file.Name.Name == "" {
		return "", false
	}
	return file.Name.Name, true
}

func parseDocumentationBeforeLine(path string, startLine int, cache map[string][]string) string {
	lines, ok := cache[path]
	if !ok {
		lines = readDocumentationLines(path)
		cache[path] = lines
	}
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
