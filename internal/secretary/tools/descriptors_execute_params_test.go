package tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestDescriptors_ParamsCoverExecuteFieldAccess(t *testing.T) {
	toolTypes := make(map[string]string, 64)
	for _, tool := range DefaultTools() {
		name := strings.TrimSpace(tool.Name())
		if name == "" {
			continue
		}
		typ := reflect.TypeOf(tool)
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ.Name() == "" {
			t.Fatalf("tool %q has unnamed type %v", name, typ)
		}
		toolTypes[name] = typ.Name()
	}

	descs := make(map[string]Descriptor, 64)
	for _, d := range Descriptors() {
		descs[strings.TrimSpace(d.Name)] = d
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("cannot locate test file path via runtime.Caller")
	}
	dir := filepath.Dir(thisFile)

	a, err := newFieldAccessAnalyzer(dir)
	if err != nil {
		t.Fatalf("new analyzer: %v", err)
	}

	for toolName, typeName := range toolTypes {
		d, ok := descs[toolName]
		if !ok {
			t.Fatalf("missing descriptor for tool %q", toolName)
		}

		keys, found, err := a.keysForMethod(typeName, "Execute")
		if err != nil {
			t.Fatalf("analyze %s.Execute: %v", typeName, err)
		}
		if !found {
			continue
		}
		if len(keys) == 0 {
			continue
		}

		if len(d.Params) == 0 {
			t.Fatalf("tool %q accesses call.Fields keys %v but declares no Params()", toolName, keys)
		}
		paramSet := make(map[string]struct{}, len(d.Params))
		for _, p := range d.Params {
			paramSet[strings.TrimSpace(p)] = struct{}{}
		}
		for _, key := range keys {
			if _, ok := paramSet[key]; !ok {
				t.Fatalf("tool %q Execute accesses call.Fields[%q] but Params() does not include it; Params=%v", toolName, key, d.Params)
			}
		}
	}
}

type fieldAccessAnalyzer struct {
	fset    *token.FileSet
	funcs   map[string]*ast.FuncDecl
	methods map[string]*ast.FuncDecl

	cache      map[string][]string
	inProgress map[string]bool
}

func newFieldAccessAnalyzer(dir string) (*fieldAccessAnalyzer, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info fs.FileInfo) bool {
		name := info.Name()
		if strings.HasSuffix(name, "_test.go") {
			return false
		}
		return strings.HasSuffix(name, ".go")
	}, 0)
	if err != nil {
		return nil, err
	}

	funcs := make(map[string]*ast.FuncDecl, 128)
	methods := make(map[string]*ast.FuncDecl, 128)

	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name == nil {
					continue
				}
				if fn.Recv == nil || len(fn.Recv.List) == 0 {
					funcs[fn.Name.Name] = fn
					continue
				}
				recvName := receiverTypeName(fn.Recv.List[0].Type)
				if recvName == "" {
					continue
				}
				methods[recvName+"."+fn.Name.Name] = fn
			}
		}
	}

	return &fieldAccessAnalyzer{
		fset:       fset,
		funcs:      funcs,
		methods:    methods,
		cache:      make(map[string][]string, 256),
		inProgress: make(map[string]bool, 256),
	}, nil
}

func receiverTypeName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

type paramKind uint8

const (
	paramOther paramKind = iota
	paramToolCall
	paramStringMap
)

type paramInfo struct {
	name string
	kind paramKind
}

func flattenParams(fn *ast.FuncDecl) []paramInfo {
	if fn == nil || fn.Type == nil || fn.Type.Params == nil {
		return nil
	}
	var out []paramInfo
	for _, field := range fn.Type.Params.List {
		kind := classifyParamType(field.Type)
		if len(field.Names) == 0 {
			out = append(out, paramInfo{name: "", kind: kind})
			continue
		}
		for _, n := range field.Names {
			out = append(out, paramInfo{name: n.Name, kind: kind})
		}
	}
	return out
}

func classifyParamType(expr ast.Expr) paramKind {
	switch x := expr.(type) {
	case *ast.SelectorExpr:
		if x.Sel != nil && x.Sel.Name == "ToolCall" {
			return paramToolCall
		}
	case *ast.MapType:
		key, okKey := x.Key.(*ast.Ident)
		val, okVal := x.Value.(*ast.Ident)
		if okKey && okVal && key.Name == "string" && val.Name == "string" {
			return paramStringMap
		}
	}
	return paramOther
}

func (a *fieldAccessAnalyzer) keysForMethod(receiverType, methodName string) ([]string, bool, error) {
	fn := a.methods[receiverType+"."+methodName]
	if fn == nil {
		return nil, false, nil
	}
	tcSeeds := make([]string, 0, 1)
	for _, p := range flattenParams(fn) {
		if p.kind == paramToolCall && strings.TrimSpace(p.name) != "" {
			tcSeeds = append(tcSeeds, p.name)
		}
	}
	keys, err := a.keysForFunc(receiverType+"."+methodName, fn, tcSeeds, nil)
	if err != nil {
		return nil, true, err
	}
	return keys, true, nil
}

func (a *fieldAccessAnalyzer) keysForFunc(fnID string, fn *ast.FuncDecl, tcSeeds, mapSeeds []string) ([]string, error) {
	tcSeeds = trimAndSort(tcSeeds)
	mapSeeds = trimAndSort(mapSeeds)
	cacheKey := fnID + "|tc=" + strings.Join(tcSeeds, ",") + "|m=" + strings.Join(mapSeeds, ",")
	if cached, ok := a.cache[cacheKey]; ok {
		return append([]string(nil), cached...), nil
	}
	if a.inProgress[cacheKey] {
		return nil, nil
	}
	a.inProgress[cacheKey] = true
	defer func() { delete(a.inProgress, cacheKey) }()

	tcAliases := make(map[string]struct{}, len(tcSeeds))
	for _, name := range tcSeeds {
		tcAliases[name] = struct{}{}
	}
	mapAliases := make(map[string]struct{}, len(mapSeeds))
	for _, name := range mapSeeds {
		mapAliases[name] = struct{}{}
	}

	keys := make(map[string]struct{}, 32)
	a.analyzeStmtList(fn.Body, tcAliases, mapAliases, keys)

	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	a.cache[cacheKey] = append([]string(nil), out...)
	return out, nil
}

func trimAndSort(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func cloneSet(in map[string]struct{}) map[string]struct{} {
	if len(in) == 0 {
		return map[string]struct{}{}
	}
	out := make(map[string]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

func (a *fieldAccessAnalyzer) analyzeStmtList(block *ast.BlockStmt, tcAliases, mapAliases map[string]struct{}, keys map[string]struct{}) {
	if block == nil {
		return
	}
	for _, stmt := range block.List {
		a.analyzeStmt(stmt, tcAliases, mapAliases, keys)
	}
}

func (a *fieldAccessAnalyzer) analyzeStmt(stmt ast.Stmt, tcAliases, mapAliases map[string]struct{}, keys map[string]struct{}) {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		a.analyzeStmtList(s, cloneSet(tcAliases), cloneSet(mapAliases), keys)
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			a.collectKeysFromExpr(rhs, tcAliases, mapAliases, keys)
		}
		a.updateAliasesFromAssign(s, tcAliases, mapAliases)
	case *ast.DeclStmt:
		decl, ok := s.Decl.(*ast.GenDecl)
		if !ok {
			return
		}
		for _, spec := range decl.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, v := range vs.Values {
				a.collectKeysFromExpr(v, tcAliases, mapAliases, keys)
			}
			a.updateAliasesFromValueSpec(vs, tcAliases, mapAliases)
		}
	case *ast.ExprStmt:
		a.collectKeysFromExpr(s.X, tcAliases, mapAliases, keys)
	case *ast.ReturnStmt:
		for _, r := range s.Results {
			a.collectKeysFromExpr(r, tcAliases, mapAliases, keys)
		}
	case *ast.IfStmt:
		scopeTC := cloneSet(tcAliases)
		scopeMap := cloneSet(mapAliases)
		if s.Init != nil {
			a.analyzeStmt(s.Init, scopeTC, scopeMap, keys)
		}
		a.collectKeysFromExpr(s.Cond, scopeTC, scopeMap, keys)
		a.analyzeStmtList(s.Body, cloneSet(scopeTC), cloneSet(scopeMap), keys)
		if s.Else != nil {
			a.analyzeStmt(s.Else, cloneSet(scopeTC), cloneSet(scopeMap), keys)
		}
	case *ast.ForStmt:
		scopeTC := cloneSet(tcAliases)
		scopeMap := cloneSet(mapAliases)
		if s.Init != nil {
			a.analyzeStmt(s.Init, scopeTC, scopeMap, keys)
		}
		if s.Cond != nil {
			a.collectKeysFromExpr(s.Cond, scopeTC, scopeMap, keys)
		}
		if s.Post != nil {
			a.analyzeStmt(s.Post, scopeTC, scopeMap, keys)
		}
		a.analyzeStmtList(s.Body, cloneSet(scopeTC), cloneSet(scopeMap), keys)
	case *ast.RangeStmt:
		scopeTC := cloneSet(tcAliases)
		scopeMap := cloneSet(mapAliases)
		a.collectKeysFromExpr(s.X, scopeTC, scopeMap, keys)
		a.analyzeStmtList(s.Body, cloneSet(scopeTC), cloneSet(scopeMap), keys)
	case *ast.SwitchStmt:
		scopeTC := cloneSet(tcAliases)
		scopeMap := cloneSet(mapAliases)
		if s.Init != nil {
			a.analyzeStmt(s.Init, scopeTC, scopeMap, keys)
		}
		if s.Tag != nil {
			a.collectKeysFromExpr(s.Tag, scopeTC, scopeMap, keys)
		}
		for _, c := range s.Body.List {
			cc, ok := c.(*ast.CaseClause)
			if !ok {
				continue
			}
			caseTC := cloneSet(scopeTC)
			caseMap := cloneSet(scopeMap)
			for _, e := range cc.List {
				a.collectKeysFromExpr(e, caseTC, caseMap, keys)
			}
			for _, st := range cc.Body {
				a.analyzeStmt(st, caseTC, caseMap, keys)
			}
		}
	default:
	}
}

func (a *fieldAccessAnalyzer) updateAliasesFromAssign(stmt *ast.AssignStmt, tcAliases, mapAliases map[string]struct{}) {
	if stmt == nil {
		return
	}
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	for i, lhs := range stmt.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok || strings.TrimSpace(id.Name) == "" {
			continue
		}
		kind := a.aliasKindForExpr(stmt.Rhs[i], tcAliases, mapAliases)
		switch kind {
		case paramToolCall:
			tcAliases[id.Name] = struct{}{}
			delete(mapAliases, id.Name)
		case paramStringMap:
			mapAliases[id.Name] = struct{}{}
			delete(tcAliases, id.Name)
		default:
			delete(tcAliases, id.Name)
			delete(mapAliases, id.Name)
		}
	}
}

func (a *fieldAccessAnalyzer) updateAliasesFromValueSpec(vs *ast.ValueSpec, tcAliases, mapAliases map[string]struct{}) {
	if vs == nil {
		return
	}
	if len(vs.Names) != len(vs.Values) {
		return
	}
	for i, n := range vs.Names {
		name := strings.TrimSpace(n.Name)
		if name == "" {
			continue
		}
		kind := a.aliasKindForExpr(vs.Values[i], tcAliases, mapAliases)
		switch kind {
		case paramToolCall:
			tcAliases[name] = struct{}{}
			delete(mapAliases, name)
		case paramStringMap:
			mapAliases[name] = struct{}{}
			delete(tcAliases, name)
		default:
			delete(tcAliases, name)
			delete(mapAliases, name)
		}
	}
}

func (a *fieldAccessAnalyzer) aliasKindForExpr(expr ast.Expr, tcAliases, mapAliases map[string]struct{}) paramKind {
	switch x := expr.(type) {
	case *ast.Ident:
		if _, ok := tcAliases[x.Name]; ok {
			return paramToolCall
		}
		if _, ok := mapAliases[x.Name]; ok {
			return paramStringMap
		}
	case *ast.SelectorExpr:
		if x.Sel == nil || x.Sel.Name != "Fields" {
			return paramOther
		}
		id, ok := x.X.(*ast.Ident)
		if !ok {
			return paramOther
		}
		if _, ok := tcAliases[id.Name]; ok {
			return paramStringMap
		}
	}
	return paramOther
}

func (a *fieldAccessAnalyzer) collectKeysFromExpr(expr ast.Expr, tcAliases, mapAliases map[string]struct{}, keys map[string]struct{}) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IndexExpr:
			if key := extractStringIndexKey(x.Index); key != "" {
				switch base := x.X.(type) {
				case *ast.Ident:
					if _, ok := mapAliases[base.Name]; ok {
						keys[key] = struct{}{}
					}
				case *ast.SelectorExpr:
					if base.Sel != nil && base.Sel.Name == "Fields" {
						if id, ok := base.X.(*ast.Ident); ok {
							if _, ok := tcAliases[id.Name]; ok {
								keys[key] = struct{}{}
							}
						}
					}
				}
			}
		case *ast.CallExpr:
			a.collectKeysFromCall(x, tcAliases, mapAliases, keys)
		}
		return true
	})
}

func (a *fieldAccessAnalyzer) collectKeysFromCall(call *ast.CallExpr, tcAliases, mapAliases map[string]struct{}, keys map[string]struct{}) {
	if call == nil {
		return
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || strings.TrimSpace(id.Name) == "" {
		return
	}
	callee := a.funcs[id.Name]
	if callee == nil {
		return
	}
	params := flattenParams(callee)
	if len(params) == 0 || len(call.Args) == 0 {
		return
	}

	var (
		tcSeeds  []string
		mapSeeds []string
	)
	for i, arg := range call.Args {
		if i >= len(params) {
			break
		}
		p := params[i]
		if strings.TrimSpace(p.name) == "" {
			continue
		}
		switch p.kind {
		case paramToolCall:
			if argID, ok := arg.(*ast.Ident); ok {
				if _, ok := tcAliases[argID.Name]; ok {
					tcSeeds = append(tcSeeds, p.name)
				}
			}
		case paramStringMap:
			switch ax := arg.(type) {
			case *ast.Ident:
				if _, ok := mapAliases[ax.Name]; ok {
					mapSeeds = append(mapSeeds, p.name)
				}
			case *ast.SelectorExpr:
				if ax.Sel != nil && ax.Sel.Name == "Fields" {
					if base, ok := ax.X.(*ast.Ident); ok {
						if _, ok := tcAliases[base.Name]; ok {
							mapSeeds = append(mapSeeds, p.name)
						}
					}
				}
			}
		}
	}
	if len(tcSeeds) == 0 && len(mapSeeds) == 0 {
		return
	}
	out, err := a.keysForFunc(id.Name, callee, tcSeeds, mapSeeds)
	if err != nil {
		return
	}
	for _, k := range out {
		keys[k] = struct{}{}
	}
}

func extractStringIndexKey(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}
