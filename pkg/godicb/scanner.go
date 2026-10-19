package godicb

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ScanOptions defines the directories whose Go files are searched for godicb
// directives. Relative paths are resolved from the process working directory.
type ScanOptions struct {
	Root      string
	WithDirs  []string
	WithTrees []string
}

// Directive describes a //di: directive attached to a Go declaration.
type Directive struct {
	File       string
	Line       int
	Kind       DeclarationKind
	Name       string
	Value      string
	Parameters []Parameter
	Results    []string
}

// Parameter describes a named or unnamed function parameter.
type Parameter struct {
	Name string
	Type string
}

// DeclarationKind identifies the kind of declaration annotated by a directive.
type DeclarationKind string

const (
	FunctionDeclaration DeclarationKind = "func"
)

// Scan parses Go files in the scan root and any explicitly included paths,
// returning directives in deterministic file and source order.
func Scan(options ScanOptions) ([]Directive, error) {
	files, err := inputGoFiles(options)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	var directives []Directive
	for _, filename := range files {
		file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments|parser.AllErrors)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filename, err)
		}
		found, err := directivesInFile(fset, file, filename)
		if err != nil {
			return nil, err
		}
		directives = append(directives, found...)
	}

	sort.Slice(directives, func(i, j int) bool {
		if directives[i].File != directives[j].File {
			return directives[i].File < directives[j].File
		}
		return directives[i].Line < directives[j].Line
	})
	return directives, nil
}

func absoluteDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return abs, nil
}

func collectDirectory(dir string, files *[]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	hasGoFile := false
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			*files = append(*files, filepath.Join(dir, entry.Name()))
			hasGoFile = true
		}
	}
	if !hasGoFile {
		return fmt.Errorf("%s contains no Go files", dir)
	}
	return nil
}

func directivesInFile(fset *token.FileSet, file *ast.File, filename string) ([]Directive, error) {
	var directives []Directive
	seen := make(map[token.Pos]struct{})

	addGroup := func(group *ast.CommentGroup, kind DeclarationKind, name string, parameters []Parameter, results []string) error {
		if group == nil {
			return nil
		}
		for _, comment := range group.List {
			text := strings.TrimSpace(comment.Text)
			if !strings.HasPrefix(text, "//di:") {
				if text != "//di" && !strings.HasPrefix(text, "//di ") && !strings.HasPrefix(text, "//di\t") {
					continue
				}
				position := fset.Position(comment.Pos())
				return fmt.Errorf("%s:%d: malformed directive %q; expected //di:<value>", position.Filename, position.Line, text)
			}
			if kind != FunctionDeclaration {
				position := fset.Position(comment.Pos())
				return fmt.Errorf("%s:%d: //di: directives can only annotate constructor/factory functions", position.Filename, position.Line)
			}
			if _, exists := seen[comment.Pos()]; exists {
				continue
			}
			value := strings.TrimSpace(strings.TrimPrefix(text, "//di:"))
			if value == "" {
				position := fset.Position(comment.Pos())
				return fmt.Errorf("%s:%d: empty //di: directive", position.Filename, position.Line)
			}
			position := fset.Position(comment.Pos())
			directives = append(directives, Directive{
				File:       filename,
				Line:       position.Line,
				Kind:       kind,
				Name:       name,
				Value:      value,
				Parameters: append([]Parameter(nil), parameters...),
				Results:    append([]string(nil), results...),
			})
			seen[comment.Pos()] = struct{}{}
		}
		return nil
	}

	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			parameters, err := functionParameters(fset, file, decl.Type.Params)
			if err != nil {
				return nil, fmt.Errorf("%s: format parameters of %s: %w", filename, decl.Name.Name, err)
			}
			results, err := functionResults(fset, file, decl.Type.Results)
			if err != nil {
				return nil, fmt.Errorf("%s: format results of %s: %w", filename, decl.Name.Name, err)
			}
			if err := addGroup(decl.Doc, FunctionDeclaration, decl.Name.Name, parameters, results); err != nil {
				return nil, err
			}
		case *ast.GenDecl:
			if err := addGroup(decl.Doc, "", "", nil, nil); err != nil {
				return nil, err
			}
			for _, spec := range decl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					if err := addGroup(typeSpec.Doc, "", typeSpec.Name.Name, nil, nil); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return directives, nil
}

func functionParameters(fset *token.FileSet, file *ast.File, fields *ast.FieldList) ([]Parameter, error) {
	if fields == nil {
		return nil, nil
	}
	imports := importPaths(file)

	var parameters []Parameter
	for _, field := range fields.List {
		qualifyImportedSelectors(field.Type, imports)
		var typeBuffer bytes.Buffer
		if err := format.Node(&typeBuffer, fset, field.Type); err != nil {
			return nil, err
		}
		if len(field.Names) == 0 {
			parameters = append(parameters, Parameter{Type: typeBuffer.String()})
			continue
		}
		for _, name := range field.Names {
			parameters = append(parameters, Parameter{
				Name: name.Name,
				Type: typeBuffer.String(),
			})
		}
	}
	return parameters, nil
}

func functionResults(fset *token.FileSet, file *ast.File, fields *ast.FieldList) ([]string, error) {
	if fields == nil {
		return nil, nil
	}
	imports := importPaths(file)
	var results []string
	for _, field := range fields.List {
		qualifyImportedSelectors(field.Type, imports)
		var typeBuffer bytes.Buffer
		if err := format.Node(&typeBuffer, fset, field.Type); err != nil {
			return nil, err
		}
		for range field.Names {
			results = append(results, typeBuffer.String())
		}
		if len(field.Names) == 0 {
			results = append(results, typeBuffer.String())
		}
	}
	return results, nil
}

func qualifyImportedSelectors(expr ast.Expr, imports map[string]string) {
	ast.Inspect(expr, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if path, exists := imports[pkg.Name]; exists {
			pkg.Name = path
		}
		return true
	})
}

func importPaths(file *ast.File) map[string]string {
	imports := make(map[string]string, len(file.Imports))
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name != "_" && name != "." {
			imports[name] = path
		}
	}
	return imports
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	unique := values[:1]
	for _, value := range values[1:] {
		if value != unique[len(unique)-1] {
			unique = append(unique, value)
		}
	}
	return unique
}
