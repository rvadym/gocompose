package godicb

import (
	"bytes"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// GenerateOptions configures container-file generation. It operates on an
// already-resolved DependencyGraph (see BuildDependencyGraph) rather than
// scanning again, so a single godicb invocation never parses a scan root
// more than once no matter how many outputs (report, generated file) it
// produces.
type GenerateOptions struct {
	// OutputDir overrides the directory the generated file is written to;
	// defaults to the scan root the graph was built from when empty, per
	// "the tool emits a generated Go file into the scan root directory
	// itself". Use this when the scan root's own package can't host
	// container.go without an import cycle (e.g. its root package is
	// itself imported back by the consumers it wires) -- a dedicated
	// subdirectory such as adapters/composition avoids that.
	OutputDir string
	// PackageName sets the generated file's package clause when OutputDir
	// points at a directory with no pre-existing Go files to infer it
	// from. Defaults to the directory's base name. Ignored when OutputDir
	// is empty (the scan root's existing package is used instead).
	PackageName string
	// FileName overrides the generated file's name; defaults to
	// "container.go".
	FileName string
}

// GenerateContainer renders a generated composition-root file from graph
// (as produced by BuildDependencyGraph) and writes it into the scan root
// (or OutputDir, if set). It returns the absolute path of the file
// written. Any dependency that cannot be resolved unambiguously is a hard
// error: no file is written, consistent with "fail the go generate step,
// no output file written on failure". graph is not re-scanned: this is a
// pure rendering step over data already resolved by BuildDependencyGraph.
func GenerateContainer(graph DependencyGraph, options GenerateOptions) (string, error) {
	source, outPath, err := buildContainerSource(graph, options)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(outPath, source, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", outPath, err)
	}
	return outPath, nil
}

// depsField describes one field of the generated Deps struct: an external,
// non-generatable input collected from a //di:wire constructor parameter
// that resolve() could not match to any tagged provider or constructor.
type depsField struct {
	name             string
	typ              resolvedType
	declaringPkgPath string
}

// buildContainerSource renders graph's already-resolved nodes as a
// formatted Go source file implementing Container/BuildContainer/Deps. It
// performs no parsing of its own; every type/parameter/result was already
// extracted with full fidelity (pointer-ness, import paths) by resolve().
func buildContainerSource(graph DependencyGraph, options GenerateOptions) ([]byte, string, error) {
	rg := graph.resolved
	if rg == nil {
		return nil, "", fmt.Errorf("GenerateContainer: graph was not produced by BuildDependencyGraph")
	}

	outDir := rg.scanRoot
	if options.OutputDir != "" {
		var err error
		outDir, err = filepath.Abs(options.OutputDir)
		if err != nil {
			return nil, "", fmt.Errorf("output dir: %w", err)
		}
	}
	outputRelDir, err := filepath.Rel(rg.moduleRoot, outDir)
	if err != nil {
		return nil, "", fmt.Errorf("resolve output package path: %w", err)
	}
	outputPackagePath := rg.modulePath
	if outputRelDir != "." {
		outputPackagePath += "/" + filepath.ToSlash(outputRelDir)
	}
	outputPackage, err := directoryPackageName(outDir)
	if err != nil {
		if options.OutputDir == "" {
			return nil, "", err
		}
		outputPackage = options.PackageName
		if outputPackage == "" {
			outputPackage = sanitizeIdent(filepath.Base(outDir))
		}
	}

	nodes := rg.nodes

	depsByName := make(map[string]depsField)
	depsByType := make(map[string]string) // type key -> field name, to catch same-type/different-name ambiguity

	for _, node := range nodes {
		if len(node.params) > 0 && !node.wire {
			return nil, "", fmt.Errorf("%s:%d: %s has parameters but is not tagged //di:wire", node.file, node.line, node.funcName)
		}
		if !node.wire {
			continue
		}
		for i := range node.params {
			param := &node.params[i]
			lookupKey := param.typ.lookupKey(node.pkgPath)
			switch param.source {
			case paramFromNode:
				// Already resolved by resolve(); nothing to validate here.
			case paramUnresolved:
				return nil, "", fmt.Errorf("%s:%d: %s parameter %q (%s): %s", node.file, node.line, node.funcName, param.name, lookupKey, param.reason)
			case paramFromExternal:
				if param.typ.Raw != "" {
					return nil, "", fmt.Errorf("%s:%d: %s parameter %q: unsupported external dependency type %q (slices/maps/chans/funcs are not yet supported by code generation)", node.file, node.line, node.funcName, param.name, param.typ.Raw)
				}
				existing, nameSeen := depsByName[param.name]
				// Qualify the earlier type by the package it was declared in, not
				// the current node's, or same-named local types (e.g. two
				// packages' Config) would wrongly compare equal.
				existingKey := existing.typ.lookupKey(existing.declaringPkgPath)
				if nameSeen && existingKey != lookupKey {
					return nil, "", fmt.Errorf("Deps field %q has conflicting types across constructors (%s vs %s); rename one of the parameters", param.name, existingKey, lookupKey)
				}
				if owner, typeSeen := depsByType[lookupKey]; typeSeen && owner != param.name {
					return nil, "", fmt.Errorf("external dependency type %s is used under different parameter names (%q and %q); rename one for consistency", lookupKey, owner, param.name)
				}
				depsByName[param.name] = depsField{name: param.name, typ: param.typ, declaringPkgPath: node.pkgPath}
				depsByType[lookupKey] = param.name
			}
		}
	}

	// Validate and extract each node's constructed type; deferred here
	// (rather than in resolve()) so BuildDependencyGraph's report remains
	// lenient about unconventional return shapes.
	results := make(map[*resolvedNode]resolvedType, len(nodes))
	hasError := make(map[*resolvedNode]bool, len(nodes))
	for _, node := range nodes {
		result, nodeHasError, err := node.primaryResult()
		if err != nil {
			return nil, "", fmt.Errorf("%s:%d: %w", node.file, node.line, err)
		}
		results[node] = result
		hasError[node] = nodeHasError
	}

	// Root nodes are those nothing else in this run depends on.
	usedFieldNames := make(map[string]struct{})
	for _, node := range nodes {
		if node.inDegree > 0 {
			continue
		}
		base := upperCamelFromSnake(filepath.Base(node.pkgPath))
		if base == "" {
			base = upperCamelFromSnake(node.funcName)
		}
		name := base
		for i := 2; ; i++ {
			if _, taken := usedFieldNames[name]; !taken {
				break
			}
			name = fmt.Sprintf("%s%d", base, i)
		}
		usedFieldNames[name] = struct{}{}
		node.fieldName = name
	}

	usedVarNames := make(map[string]struct{})
	for _, node := range nodes {
		// Pre-reserve the package alias each node's constructor is called
		// through, so a local variable never shadows the package it was
		// just built from (e.g. `login := login.New(...)` is legal Go but
		// confusing to read).
		usedVarNames[sanitizeIdent(filepath.Base(node.pkgPath))] = struct{}{}
	}
	resultTypeCounts := make(map[string]int)
	for _, node := range nodes {
		resultTypeCounts[results[node].Name]++
	}
	for _, node := range nodes {
		if !node.wire && len(node.provides) == 0 {
			continue // untagged/unused helper shouldn't occur, but skip defensively
		}
		base := lowerCamel(results[node].Name)
		if base == "" || strings.EqualFold(base, "interactor") ||
			resultTypeCounts[results[node].Name] > 1 {
			// Fall back to package-based naming for generic or repeated
			// result types (e.g. use cases returning *Interactor or handlers
			// from different packages sharing infra.Handler).
			base = lowerCamel(upperCamelFromSnake(filepath.Base(node.pkgPath)))
		}
		name := base
		for i := 2; ; i++ {
			if _, taken := usedVarNames[name]; !taken {
				break
			}
			name = fmt.Sprintf("%s%d", base, i)
		}
		usedVarNames[name] = struct{}{}
		node.varName = name
	}

	// Topologically order nodes so each is declared after its dependencies.
	ordered, err := topologicalOrder(nodes)
	if err != nil {
		return nil, "", err
	}

	depsFields := make([]depsField, 0, len(depsByName))
	for _, field := range depsByName {
		depsFields = append(depsFields, field)
	}
	sort.Slice(depsFields, func(i, j int) bool { return depsFields[i].name < depsFields[j].name })

	// Collect import paths and allocate explicit aliases for every one
	// referenced by the generated file.
	importPathsNeeded := map[string]struct{}{}
	for _, node := range ordered {
		if node.pkgPath != outputPackagePath {
			importPathsNeeded[node.pkgPath] = struct{}{}
		}
		if result := results[node]; result.ImportPath != "" && result.ImportPath != outputPackagePath {
			importPathsNeeded[result.ImportPath] = struct{}{}
		}
	}
	for _, field := range depsFields {
		if field.typ.ImportPath != "" {
			if field.typ.ImportPath != outputPackagePath {
				importPathsNeeded[field.typ.ImportPath] = struct{}{}
			}
		} else if field.declaringPkgPath != "" && field.declaringPkgPath != outputPackagePath && !isBuiltinType(field.typ.Name) {
			importPathsNeeded[field.declaringPkgPath] = struct{}{}
		}
	}
	needsFmt := false
	for _, node := range ordered {
		if hasError[node] {
			needsFmt = true
		}
	}
	alias := allocateAliases(importPathsNeeded)

	var body bytes.Buffer
	body.WriteString("// Code generated by godicb. DO NOT EDIT.\n\n")
	fmt.Fprintf(&body, "package %s\n\n", outputPackage)

	body.WriteString("import (\n")
	if needsFmt {
		body.WriteString("\t\"fmt\"\n\n")
	}
	type importLine struct{ alias, path string }
	var importLines []importLine
	for path := range importPathsNeeded {
		importLines = append(importLines, importLine{alias[path], path})
	}
	sort.Slice(importLines, func(i, j int) bool { return importLines[i].path < importLines[j].path })
	for _, line := range importLines {
		fmt.Fprintf(&body, "\t%s %q\n", line.alias, line.path)
	}
	body.WriteString(")\n\n")

	// Deps struct.
	body.WriteString("// Deps lists the external, non-generatable inputs this scan root needs\n")
	body.WriteString("// from the process. godicb infers this set from constructor parameters\n")
	body.WriteString("// that are not themselves resolvable interfaces or other nodes built in\n")
	body.WriteString("// this run.\n")
	body.WriteString("type Deps struct {\n")
	for _, field := range depsFields {
		typeSrc, err := field.typ.source(alias, field.declaringPkgPath, outputPackagePath)
		if err != nil {
			return nil, "", err
		}
		fmt.Fprintf(&body, "\t%s %s\n", upperCamelFromSnake(field.name), typeSrc)
	}
	body.WriteString("}\n\n")

	// Container struct.
	var roots []*resolvedNode
	for _, node := range ordered {
		if node.inDegree == 0 {
			roots = append(roots, node)
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].fieldName < roots[j].fieldName })

	body.WriteString("// Container exposes the root nodes of this scan root's resolved\n")
	body.WriteString("// dependency tree -- the nodes nothing else built in this run depends on.\n")
	body.WriteString("type Container struct {\n")
	for _, node := range roots {
		typeSrc, err := results[node].source(alias, node.pkgPath, outputPackagePath)
		if err != nil {
			return nil, "", err
		}
		fmt.Fprintf(&body, "\t%s %s\n", node.fieldName, typeSrc)
	}
	body.WriteString("}\n\n")

	// BuildContainer.
	body.WriteString("// BuildContainer constructs every node discovered in this scan root,\n")
	body.WriteString("// wiring each one into whatever depends on it, and returns the subset of\n")
	body.WriteString("// that tree left unconsumed (Container). It is the single composition\n")
	body.WriteString("// root for this scan root.\n")
	body.WriteString("func BuildContainer(deps Deps) (*Container, error) {\n")
	for _, node := range ordered {
		args := make([]string, 0, len(node.params))
		for _, param := range node.params {
			switch param.source {
			case paramFromNode:
				args = append(args, param.provider.varName)
			case paramFromExternal:
				args = append(args, "deps."+upperCamelFromSnake(param.name))
			default:
				return nil, "", fmt.Errorf("%s:%d: %s parameter %q was never resolved", node.file, node.line, node.funcName, param.name)
			}
		}
		callee := node.funcName
		if node.pkgPath != outputPackagePath {
			callee = alias[node.pkgPath] + "." + callee
		}
		call := fmt.Sprintf("%s(%s)", callee, strings.Join(args, ", "))
		if hasError[node] {
			fmt.Fprintf(&body, "\t%s, err := %s\n", node.varName, call)
			body.WriteString("\tif err != nil {\n")
			fmt.Fprintf(&body, "\t\treturn nil, fmt.Errorf(%q, err)\n", "build "+node.varName+": %w")
			body.WriteString("\t}\n")
		} else {
			fmt.Fprintf(&body, "\t%s := %s\n", node.varName, call)
		}
	}
	body.WriteString("\n\treturn &Container{\n")
	for _, node := range roots {
		fmt.Fprintf(&body, "\t\t%s: %s,\n", node.fieldName, node.varName)
	}
	body.WriteString("\t}, nil\n")
	body.WriteString("}\n")

	formatted, err := format.Source(body.Bytes())
	if err != nil {
		return nil, "", fmt.Errorf("format generated container: %w\n--- generated source ---\n%s", err, body.String())
	}

	fileName := options.FileName
	if fileName == "" {
		fileName = "container.go"
	}
	return formatted, filepath.Join(outDir, fileName), nil
}

// topologicalOrder returns nodes ordered so that every node appears after
// all the nodes its parameters resolve to, erroring on dependency cycles.
func topologicalOrder(nodes []*resolvedNode) ([]*resolvedNode, error) {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[*resolvedNode]int, len(nodes))
	var ordered []*resolvedNode
	var visit func(node *resolvedNode, path []string) error
	visit = func(node *resolvedNode, path []string) error {
		switch state[node] {
		case visited:
			return nil
		case visiting:
			return fmt.Errorf("dependency cycle detected: %s", strings.Join(append(path, node.funcName), " -> "))
		}
		state[node] = visiting
		for _, param := range node.params {
			if param.source == paramFromNode {
				if err := visit(param.provider, append(path, node.funcName)); err != nil {
					return err
				}
			}
		}
		state[node] = visited
		ordered = append(ordered, node)
		return nil
	}
	for _, node := range nodes {
		if err := visit(node, nil); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func allocateAliases(paths map[string]struct{}) map[string]string {
	sorted := make([]string, 0, len(paths))
	for path := range paths {
		sorted = append(sorted, path)
	}
	sort.Strings(sorted)

	alias := make(map[string]string, len(sorted))
	used := make(map[string]struct{})
	for _, path := range sorted {
		segments := strings.Split(path, "/")
		base := sanitizeIdent(segments[len(segments)-1])
		name := base
		depth := 2
		for i := len(segments) - 2; i >= 0 && conflicts(used, name); i-- {
			name = sanitizeIdent(segments[i]) + "_" + base
		}
		for conflicts(used, name) {
			name = fmt.Sprintf("%s%d", base, depth)
			depth++
		}
		used[name] = struct{}{}
		alias[path] = name
	}
	return alias
}

func conflicts(used map[string]struct{}, name string) bool {
	_, taken := used[name]
	return taken
}

func sanitizeIdent(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r):
			b.WriteRune(r)
		case unicode.IsDigit(r) && i > 0:
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" || unicode.IsDigit(rune(out[0])) {
		out = "pkg_" + out
	}
	return out
}

func directoryPackageName(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", dir, err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, parser.PackageClauseOnly)
		if err != nil {
			continue
		}
		return file.Name.Name, nil
	}
	return "", fmt.Errorf("%s has no Go files to determine the package name from", dir)
}

func lowerCamel(name string) string {
	if name == "" {
		return name
	}
	runes := []rune(name)
	i := 0
	for i < len(runes) && unicode.IsUpper(runes[i]) {
		i++
	}
	switch {
	case i == 0:
		return name
	case i == len(runes):
		return strings.ToLower(name)
	case i == 1:
		return strings.ToLower(string(runes[0])) + string(runes[1:])
	default:
		return strings.ToLower(string(runes[:i-1])) + string(runes[i-1:])
	}
}

func upperCamelFromSnake(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' })
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		r := []rune(part)
		b.WriteString(strings.ToUpper(string(r[0])))
		if len(r) > 1 {
			b.WriteString(string(r[1:]))
		}
	}
	return b.String()
}
