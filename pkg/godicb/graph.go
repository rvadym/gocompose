package godicb

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// DependencyGraph is a constructor dependency graph and its unresolved
// interface dependencies. It is produced by a single parse+resolve pass
// over a scan root (see resolve) and is the shared input to both the
// human-readable CLI report (FormatScanOutputCLI) and Go source generation
// (GenerateContainer) -- godicb never scans the same scan root twice to
// serve those two outputs.
type DependencyGraph struct {
	Nodes      []GraphNode
	Unresolved []UnresolvedDependency
	External   []ExternalDependency

	// resolved carries full type fidelity (pointer-ness, import paths,
	// already-resolved provider links) that GraphNode's display strings
	// deliberately discard. It is unexported: callers only ever need it
	// indirectly, by handing the DependencyGraph to GenerateContainer.
	resolved *resolvedGraph
}

// GraphNode represents one tagged constructor/factory function.
type GraphNode struct {
	File         string
	Line         int
	Function     string
	Provides     []string
	Dependencies []DependencyEdge
}

// DependencyEdge connects a constructor parameter to a provider or external
// input.
type DependencyEdge struct {
	Parameter string
	Type      string
	Provider  string
	External  bool
}

// UnresolvedDependency describes an interface parameter with no unique
// tagged provider.
type UnresolvedDependency struct {
	Function  string
	Parameter string
	Type      string
	Reason    string
}

// ExternalDependency describes a non-interface parameter supplied by the
// generated Deps struct rather than another constructor.
type ExternalDependency struct {
	Function  string
	Parameter string
	Type      string
}

// resolvedType is a code-generation-friendly description of a parameter or
// result type: either a simple (possibly pointer) identifier/selector, or,
// for unsupported composite shapes (slices, maps, chans, funcs), a raw
// fallback that code generation rejects with an actionable error rather
// than emit code that may not compile. It carries enough structure to
// serve both the human-readable report (displayString) and compilable Go
// source (source), so the AST only needs to be walked once per file.
type resolvedType struct {
	Pointer    bool
	ImportPath string // "" => builtin, or declared in a node's own package.
	Name       string
	Raw        string // set only for unsupported composite shapes.
}

// lookupKey identifies this type for provider/concrete-constructor
// matching, independent of pointer-ness or display formatting.
// declaringPkgPath is the package the type's constructor/parameter was
// declared in, used to qualify a bare (same-package) identifier.
func (t resolvedType) lookupKey(declaringPkgPath string) string {
	switch {
	case t.ImportPath != "":
		return t.ImportPath + "." + t.Name
	case isBuiltinType(t.Name):
		return t.Name
	case t.Raw != "":
		return t.Raw
	default:
		return declaringPkgPath + "." + t.Name
	}
}

// displayString renders this type the way the human-readable CLI report
// wants it: fully qualified, no pointer marker (interfaces are never
// pointers, and the report never needs one for other types either).
func (t resolvedType) displayString(declaringPkgPath string) string {
	if t.Raw != "" {
		return t.Raw
	}
	if t.ImportPath != "" {
		return t.ImportPath + "." + t.Name
	}
	if isBuiltinType(t.Name) || t.Name == "" {
		return t.Name
	}
	return declaringPkgPath + "." + t.Name
}

// source renders t as compilable Go source, qualifying it with the
// generated file's chosen import alias. declaringPkgPath is the package
// the type is declared in when t.ImportPath is empty (i.e. it was a bare
// identifier in its constructor's own file). outputPkgPath is the package
// the generated file itself belongs to, so same-package types are left
// unqualified.
func (t resolvedType) source(alias map[string]string, declaringPkgPath, outputPkgPath string) (string, error) {
	if t.Raw != "" {
		return "", fmt.Errorf("unsupported type %q for code generation (slices/maps/chans/funcs are not yet supported; use a named type)", t.Raw)
	}
	name := t.Name
	switch {
	case t.ImportPath != "":
		name = alias[t.ImportPath] + "." + t.Name
	case isBuiltinType(t.Name):
		// unqualified builtin, nothing to do.
	case declaringPkgPath != "" && declaringPkgPath != outputPkgPath:
		name = alias[declaringPkgPath] + "." + t.Name
	}
	if t.Pointer {
		return "*" + name, nil
	}
	return name, nil
}

// paramSource classifies how a //di:wire constructor parameter was
// resolved.
type paramSource int

const (
	paramUnresolved   paramSource = iota
	paramFromNode                 // resolved to another tagged node in this graph.
	paramFromExternal             // destined for the generated Deps struct.
)

// resolvedParam is a single constructor parameter after resolution.
type resolvedParam struct {
	name     string
	typ      resolvedType
	source   paramSource
	provider *resolvedNode // set when source == paramFromNode.
	reason   string        // set when source == paramUnresolved.
}

// resolvedNode is the fully-resolved internal representation of one tagged
// constructor/factory function, computed once per scan root and shared by
// both DependencyGraph's display projection and GenerateContainer's Go
// source projection.
type resolvedNode struct {
	key      string
	file     string
	line     int
	pkgPath  string
	funcName string
	wire     bool
	provides []string
	params   []resolvedParam
	results  []resolvedType // 0, 1 (T), or 2 (T, error) as declared; unvalidated here.
	inDegree int

	// assigned only during Go source generation (see generate.go):
	varName   string
	fieldName string
}

// primaryResult validates and extracts this node's constructed type,
// enforcing the "func Name(deps...) T" / "func Name(deps...) (T, error)"
// constructor shape. This check only matters for source generation --
// BuildDependencyGraph's report never calls it, so a function tagged
// //di:wire purely for a report/diagnostic walk-through is never rejected
// just for having an unconventional return shape.
func (n *resolvedNode) primaryResult() (resolvedType, bool, error) {
	switch len(n.results) {
	case 1:
		if n.results[0].ImportPath == "" && n.results[0].Name == "error" {
			return resolvedType{}, false, fmt.Errorf("%s: constructor must return a constructed value, not only error", n.funcName)
		}
		return n.results[0], false, nil
	case 2:
		if n.results[1].ImportPath != "" || n.results[1].Name != "error" {
			return resolvedType{}, false, fmt.Errorf("%s: constructor's second return value must be error", n.funcName)
		}
		return n.results[0], true, nil
	default:
		return resolvedType{}, false, fmt.Errorf("%s: constructor must return (T) or (T, error)", n.funcName)
	}
}

// resolvedGraph is the complete result of a single parse+resolve pass over
// a scan root.
type resolvedGraph struct {
	nodes      []*resolvedNode // sorted by key.
	moduleRoot string
	modulePath string
	scanRoot   string // absolute scan root directory (options.Root).
}

// BuildDependencyGraph discovers tagged functions and connects their
// interface dependencies to uniquely tagged providers. It parses and
// resolves the scan root exactly once; pass the returned DependencyGraph to
// GenerateContainer to render Go source from the same resolution without
// scanning again.
func BuildDependencyGraph(options ScanOptions) (DependencyGraph, error) {
	rg, err := resolve(options)
	if err != nil {
		return DependencyGraph{}, err
	}
	return rg.toDependencyGraph(), nil
}

// resolve parses every Go file in the scan root exactly once, collects
// declared interfaces and //di: tagged constructors with full type
// fidelity, and resolves every //di:wire parameter to either a uniquely
// tagged provider, another tagged node (concrete-to-concrete wiring), or
// "no unique match" (recorded, not failed -- see resolvedParam.reason).
func resolve(options ScanOptions) (*resolvedGraph, error) {
	root := options.Root
	if root == "" {
		root = "."
	}
	rootAbs, err := absoluteDirectory(root)
	if err != nil {
		return nil, fmt.Errorf("scan root: %w", err)
	}
	files, err := inputGoFiles(options)
	if err != nil {
		return nil, err
	}
	moduleRoot, modulePath, err := findModule(rootAbs)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	nodesByKey := make(map[string]*resolvedNode)
	interfaceTypes := make(map[string]struct{})
	interfaceNames := make(map[string][]string)

	for _, filename := range files {
		file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments|parser.AllErrors)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filename, err)
		}
		relDir, err := filepath.Rel(moduleRoot, filepath.Dir(filename))
		if err != nil {
			return nil, fmt.Errorf("resolve package path for %s: %w", filename, err)
		}
		pkgPath := modulePath
		if relDir != "." {
			pkgPath += "/" + filepath.ToSlash(relDir)
		}

		collectInterfaceTypes(file, pkgPath, interfaceTypes, interfaceNames)

		// Extract full-fidelity (pointer-ness, import path) parameter/
		// result shapes directly from the as-yet-unmodified AST, before
		// calling directivesInFile below: directivesInFile's
		// functionParameters/functionResults helpers rewrite selector
		// expressions in place (qualifyImportedSelectors) to produce their
		// own display-only strings, which would otherwise corrupt the
		// very same *ast.File nodes constructorShape needs to read.
		imports := importPaths(file)
		shapes := make(map[string]struct {
			params  []resolvedParam
			results []resolvedType
		})
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			params, results, err := constructorShape(fset, fn, imports)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", filename, err)
			}
			shapes[fn.Name.Name] = struct {
				params  []resolvedParam
				results []resolvedType
			}{params, results}
		}

		directives, err := directivesInFile(fset, file, filename)
		if err != nil {
			return nil, err
		}
		if len(directives) == 0 {
			continue
		}

		for _, directive := range directives {
			key := directive.File + ":" + directive.Name
			node := nodesByKey[key]
			if node == nil {
				shape, ok := shapes[directive.Name]
				if !ok {
					return nil, fmt.Errorf("%s:%d: internal error: directive's function declaration not found", filename, directive.Line)
				}
				node = &resolvedNode{
					key:      key,
					file:     directive.File,
					line:     directive.Line,
					pkgPath:  pkgPath,
					funcName: directive.Name,
					params:   shape.params,
					results:  shape.results,
				}
				nodesByKey[key] = node
			}
			target, _, _ := strings.Cut(directive.Value, "@")
			if target == "wire" {
				node.wire = true
			} else {
				node.provides = append(node.provides, target)
			}
		}
	}

	nodes := make([]*resolvedNode, 0, len(nodesByKey))
	for _, node := range nodesByKey {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].key < nodes[j].key })

	providers := make(map[string][]*resolvedNode)
	concreteConstructors := make(map[string][]*resolvedNode)
	for _, node := range nodes {
		for _, provided := range node.provides {
			interfacePath, err := resolveInterfaceTag(provided, interfaceNames)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", node.file, node.line, err)
			}
			providers[interfacePath] = append(providers[interfacePath], node)
		}
		for _, result := range node.results {
			resultKey := result.lookupKey(node.pkgPath)
			concreteConstructors[resultKey] = append(concreteConstructors[resultKey], node)
		}
	}

	for _, node := range nodes {
		if !node.wire {
			continue
		}
		for i := range node.params {
			param := &node.params[i]
			lookupKey := param.typ.lookupKey(node.pkgPath)
			if _, isInterface := interfaceTypes[lookupKey]; isInterface {
				candidates := providers[lookupKey]
				switch len(candidates) {
				case 0:
					param.source = paramUnresolved
					param.reason = "no tagged provider"
				case 1:
					param.source = paramFromNode
					param.provider = candidates[0]
					candidates[0].inDegree++
				default:
					param.source = paramUnresolved
					param.reason = fmt.Sprintf("%d tagged providers", len(candidates))
				}
				continue
			}
			if candidates := concreteConstructors[lookupKey]; len(candidates) == 1 {
				param.source = paramFromNode
				param.provider = candidates[0]
				candidates[0].inDegree++
			} else if len(candidates) > 1 {
				param.source = paramUnresolved
				param.reason = fmt.Sprintf("%d tagged constructors produce this type", len(candidates))
			} else {
				param.source = paramFromExternal
			}
		}
	}

	return &resolvedGraph{
		nodes:      nodes,
		moduleRoot: moduleRoot,
		modulePath: modulePath,
		scanRoot:   rootAbs,
	}, nil
}

// toDependencyGraph projects a resolvedGraph into the human-readable,
// string-typed DependencyGraph consumed by FormatScanOutputCLI. This is a
// pure projection: no parsing or resolution happens here.
func (rg *resolvedGraph) toDependencyGraph() DependencyGraph {
	graph := DependencyGraph{resolved: rg}
	for _, node := range rg.nodes {
		display := GraphNode{
			File:     node.file,
			Line:     node.line,
			Function: node.pkgPath + "." + node.funcName,
			Provides: append([]string(nil), node.provides...),
		}
		if node.wire {
			for _, param := range node.params {
				dependencyType := param.typ.displayString(node.pkgPath)
				label := param.name
				if label == "" {
					label = "<unnamed>"
				}
				switch param.source {
				case paramFromNode:
					display.Dependencies = append(display.Dependencies, DependencyEdge{
						Parameter: label,
						Type:      dependencyType,
						Provider:  param.provider.pkgPath + "." + param.provider.funcName,
					})
				case paramFromExternal:
					graph.External = append(graph.External, ExternalDependency{
						Function:  node.pkgPath + "." + node.funcName,
						Parameter: label,
						Type:      dependencyType,
					})
				default:
					graph.Unresolved = append(graph.Unresolved, UnresolvedDependency{
						Function:  node.pkgPath + "." + node.funcName,
						Parameter: label,
						Type:      dependencyType,
						Reason:    param.reason,
					})
				}
			}
		}
		graph.Nodes = append(graph.Nodes, display)
	}
	sort.Slice(graph.Unresolved, func(i, j int) bool {
		if graph.Unresolved[i].Function != graph.Unresolved[j].Function {
			return graph.Unresolved[i].Function < graph.Unresolved[j].Function
		}
		return graph.Unresolved[i].Parameter < graph.Unresolved[j].Parameter
	})
	sort.Slice(graph.External, func(i, j int) bool {
		if graph.External[i].Function != graph.External[j].Function {
			return graph.External[i].Function < graph.External[j].Function
		}
		return graph.External[i].Parameter < graph.External[j].Parameter
	})
	return graph
}

// collectInterfaceTypes records every `type X interface {...}` declared in
// file into the shared interfaceTypes/interfaceNames maps.
func collectInterfaceTypes(file *ast.File, pkgPath string, interfaceTypes map[string]struct{}, interfaceNames map[string][]string) {
	for _, declaration := range file.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if _, ok := typeSpec.Type.(*ast.InterfaceType); ok {
				interfacePath := pkgPath + "." + typeSpec.Name.Name
				interfaceTypes[interfacePath] = struct{}{}
				interfaceNames[typeSpec.Name.Name] = append(interfaceNames[typeSpec.Name.Name], interfacePath)
			}
		}
	}
}

// constructorShape extracts resolvedType-fidelity parameter/result
// information directly from a tagged function's AST (pointer-ness and
// import paths that the scanner package's pre-formatted display strings
// deliberately discard). fn is nil only if a directive's declaring
// function could not be found, which should not happen given
// directivesInFile already requires a *ast.FuncDecl target.
func constructorShape(fset *token.FileSet, fn *ast.FuncDecl, imports map[string]string) ([]resolvedParam, []resolvedType, error) {
	if fn == nil {
		return nil, nil, fmt.Errorf("internal error: directive's function declaration not found")
	}
	var params []resolvedParam
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			typ := newResolvedType(fset, field.Type, imports)
			if len(field.Names) == 0 {
				params = append(params, resolvedParam{typ: typ})
				continue
			}
			for _, name := range field.Names {
				params = append(params, resolvedParam{name: name.Name, typ: typ})
			}
		}
	}

	var results []resolvedType
	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			typ := newResolvedType(fset, field.Type, imports)
			count := len(field.Names)
			if count == 0 {
				count = 1
			}
			for i := 0; i < count; i++ {
				results = append(results, typ)
			}
		}
	}
	return params, results, nil
}

func newResolvedType(fset *token.FileSet, expr ast.Expr, imports map[string]string) resolvedType {
	pointer := false
	for {
		star, ok := expr.(*ast.StarExpr)
		if !ok {
			break
		}
		pointer = true
		expr = star.X
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return resolvedType{Pointer: pointer, Name: e.Name}
	case *ast.SelectorExpr:
		if pkgIdent, ok := e.X.(*ast.Ident); ok {
			if path, exists := imports[pkgIdent.Name]; exists {
				return resolvedType{Pointer: pointer, ImportPath: path, Name: e.Sel.Name}
			}
		}
	}
	var buf bytes.Buffer
	prefix := ""
	if pointer {
		prefix = "*"
	}
	_ = format.Node(&buf, fset, expr)
	return resolvedType{Raw: prefix + buf.String()}
}

func resolveInterfaceTag(tag string, names map[string][]string) (string, error) {
	if strings.Contains(tag, "/") {
		return tag, nil
	}
	candidates := names[tag]
	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("interface tag %q does not match an interface in the scanned Go packages", tag)
	case 1:
		return candidates[0], nil
	default:
		return "", fmt.Errorf("interface tag %q is ambiguous; use a fully qualified //di:<import/path>.%s tag", tag, tag)
	}
}

func inputGoFiles(options ScanOptions) ([]string, error) {
	root := options.Root
	if root == "" {
		root = "."
	}
	root, err := absoluteDirectory(root)
	if err != nil {
		return nil, fmt.Errorf("scan root: %w", err)
	}
	var files []string
	if err := appendTreeFiles(root, &files); err != nil {
		return nil, err
	}
	for _, path := range options.WithDirs {
		dir, err := absoluteDirectory(path)
		if err != nil {
			return nil, fmt.Errorf("--with-dir=%s: %w", path, err)
		}
		if err := collectDirectory(dir, &files); err != nil {
			return nil, fmt.Errorf("--with-dir=%s: %w", path, err)
		}
	}
	for _, path := range options.WithTrees {
		dir, err := absoluteDirectory(path)
		if err != nil {
			return nil, fmt.Errorf("--with-tree=%s: %w", path, err)
		}
		if err := appendTreeFiles(dir, &files); err != nil {
			return nil, fmt.Errorf("--with-tree=%s: %w", path, err)
		}
	}
	sort.Strings(files)
	return uniqueStrings(files), nil
}

func appendTreeFiles(root string, files *[]string) error {
	foundGoFile := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		*files = append(*files, path)
		foundGoFile = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk %s: %w", root, err)
	}
	if !foundGoFile {
		return fmt.Errorf("%s contains no Go files", root)
	}
	return nil
}

func isBuiltinType(typeName string) bool {
	switch typeName {
	case "any", "bool", "byte", "comparable", "complex64", "complex128",
		"error", "float32", "float64", "int", "int8", "int16", "int32",
		"int64", "rune", "string", "uint", "uint8", "uint16", "uint32",
		"uint64", "uintptr":
		return true
	default:
		return false
	}
}
