package godi

import (
	"bytes"
	"fmt"
	"go/build"
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rvadym/gocompose/pkg/internal/fs"
	"github.com/spf13/viper"
)

// bindingsFileName is the YAML file, located in an app's composition
// directory, that binds third-party constructors to app-declared interfaces.
const bindingsFileName = "godi.yaml"

// Binding wires a constructor that godi cannot annotate (e.g. it lives in a
// third-party module) to an interface declared in the same app.
//
// Both fields use the "<import/path>:<Name>" notation, for example
// "github.com/app/ports.Repository" and "github.com/vendor/impl.NewRepository",
// the same notation as //di:<import/path>.<Name> annotations.
type Binding struct {
	Interface   string `mapstructure:"interface"`
	Constructor string `mapstructure:"constructor"`
}

// LoadBindings reads the YAML bindings file at path. A missing file means the
// app has no bindings and yields a nil slice.
func LoadBindings(fileSystem fs.FileSystem, path string) ([]Binding, error) {
	if !fileSystem.Exists(path) {
		return nil, nil
	}
	data, err := fileSystem.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var bindings []Binding
	if err := v.UnmarshalKey("bindings", &bindings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	seen := make(map[string]struct{}, len(bindings))
	for i, binding := range bindings {
		if _, _, err := splitQualified(binding.Interface); err != nil {
			return nil, fmt.Errorf("%s: bindings[%d].interface: %w", path, i, err)
		}
		if _, _, err := splitQualified(binding.Constructor); err != nil {
			return nil, fmt.Errorf("%s: bindings[%d].constructor: %w", path, i, err)
		}
		if _, dup := seen[binding.Interface]; dup {
			return nil, fmt.Errorf("%s: bindings[%d]: interface %s is bound more than once", path, i, binding.Interface)
		}
		seen[binding.Interface] = struct{}{}
	}
	return bindings, nil
}

// BindInterface records binding in the godi.yaml of app, creating the file if
// needed, and returns the file path. Constructor and interface validity is
// checked against the source later, at generation time.
func (r *Runner) BindInterface(projectPath, appsPath, app string, binding Binding) (string, error) {
	projectPath, appsPath, apps, err := r.Normalize(projectPath, appsPath, []string{app})
	if err != nil {
		return "", err
	}
	if err := r.Validate(projectPath, appsPath, apps); err != nil {
		return "", err
	}
	appName := r.fileSystem.Base(apps[0])
	path := filepath.Join(appsPath, appName, "adapters", "composition", bindingsFileName)
	return path, addBinding(r.fileSystem, path, binding)
}

func addBinding(fileSystem fs.FileSystem, path string, binding Binding) error {
	if _, _, err := splitQualified(binding.Interface); err != nil {
		return fmt.Errorf("interface: %w", err)
	}
	if _, _, err := splitQualified(binding.Constructor); err != nil {
		return fmt.Errorf("constructor: %w", err)
	}
	existing, err := LoadBindings(fileSystem, path)
	if err != nil {
		return err
	}

	entries := make([]map[string]string, 0, len(existing)+1)
	for _, b := range existing {
		if b.Interface == binding.Interface {
			return fmt.Errorf("%s: interface %s is already bound", path, b.Interface)
		}
		entries = append(entries, map[string]string{"interface": b.Interface, "constructor": b.Constructor})
	}
	entries = append(entries, map[string]string{"interface": binding.Interface, "constructor": binding.Constructor})

	v := viper.New()
	v.SetConfigType("yaml")
	v.Set("bindings", entries)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// applyBindingPrecedence makes a godi.yaml binding the only provider of its
// interface when annotated providers exist too. Each displaced annotated
// provider is reported in the returned warnings, sorted by interface.
func applyBindingPrecedence(providers map[string][]*resolvedNode) []string {
	interfaces := make([]string, 0, len(providers))
	for iface := range providers {
		interfaces = append(interfaces, iface)
	}
	sort.Strings(interfaces)

	var warnings []string
	for _, iface := range interfaces {
		candidates := providers[iface]
		var binding *resolvedNode
		for _, candidate := range candidates {
			if candidate.bindsInterface != "" {
				binding = candidate
				break
			}
		}
		if binding == nil {
			continue
		}

		kept := make([]*resolvedNode, 0, 1)
		for _, candidate := range candidates {
			if candidate.bindsInterface != "" {
				kept = append(kept, candidate)
				continue
			}
			warnings = append(warnings, fmt.Sprintf(
				"%s: annotated provider %s (%s:%d) is overridden by godi.yaml binding to %s",
				iface, candidate.pkgPath+"."+candidate.funcName, candidate.file, candidate.line,
				binding.pkgPath+"."+binding.funcName,
			))
		}
		providers[iface] = kept
	}
	return warnings
}

func splitQualified(s string) (string, string, error) {
	// Same notation as //di: annotations. Go identifiers cannot contain dots,
	// so the last dot after the last slash separates path from name.
	idx := strings.LastIndex(s, ".")
	if idx <= strings.LastIndex(s, "/") || idx == len(s)-1 {
		return "", "", fmt.Errorf("%q: expected <import/path>.<Name>", s)
	}
	pkgPath, name := s[:idx], s[idx+1:]
	if !token.IsIdentifier(name) {
		return "", "", fmt.Errorf("%q: %q is not a valid identifier", s, name)
	}
	return pkgPath, name, nil
}

// bindingNodes turns each binding into a synthetic constructor node. The
// constructor's signature is read from its type-checked package, which lets
// the generator wire its parameters like any tagged constructor. Each
// binding's constructor must return a value implementing the interface, and
// the interface must be declared in the scanned app (interfaceTypes).
func bindingNodes(bindings []Binding, interfaceTypes map[string]struct{}, srcDir string) ([]*resolvedNode, error) {
	if len(bindings) == 0 {
		return nil, nil
	}
	loader, release, err := newTypeLoader(srcDir)
	if err != nil {
		return nil, err
	}
	defer release()

	nodes := make([]*resolvedNode, 0, len(bindings))
	for _, binding := range bindings {
		ifacePkg, ifaceName, err := splitQualified(binding.Interface)
		if err != nil {
			return nil, err
		}
		ifaceKey := ifacePkg + "." + ifaceName
		if _, declared := interfaceTypes[ifaceKey]; !declared {
			return nil, fmt.Errorf("binding %s: interface is not declared in this app", binding.Interface)
		}
		iface, err := loader.interfaceType(ifacePkg, ifaceName)
		if err != nil {
			return nil, fmt.Errorf("binding %s: %w", binding.Interface, err)
		}

		ctorPkg, ctorName, err := splitQualified(binding.Constructor)
		if err != nil {
			return nil, err
		}
		sig, err := loader.constructorSignature(ctorPkg, ctorName)
		if err != nil {
			return nil, fmt.Errorf("binding %s: %w", binding.Interface, err)
		}
		resultType, err := bindingResult(sig)
		if err != nil {
			return nil, fmt.Errorf("binding %s: constructor %s: %w", binding.Interface, binding.Constructor, err)
		}
		if !types.Implements(resultType, iface) {
			return nil, fmt.Errorf("binding %s: constructor %s returns %s, which does not implement the interface", binding.Interface, binding.Constructor, resultType)
		}

		node := &resolvedNode{
			key:            "binding:" + ifaceKey,
			pkgPath:        ctorPkg,
			funcName:       ctorName,
			wire:           true,
			bindsInterface: ifaceKey,
		}
		for i := 0; i < sig.Params().Len(); i++ {
			param := sig.Params().At(i)
			name := param.Name()
			if name == "" {
				name = fmt.Sprintf("arg%d", i)
			}
			node.params = append(node.params, resolvedParam{name: name, typ: toResolvedType(param.Type())})
		}
		for i := 0; i < sig.Results().Len(); i++ {
			node.results = append(node.results, toResolvedType(sig.Results().At(i).Type()))
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// bindingResult returns the constructed type of a "T" or "(T, error)" signature.
func bindingResult(sig *types.Signature) (types.Type, error) {
	results := sig.Results()
	switch results.Len() {
	case 1:
		return results.At(0).Type(), nil
	case 2:
		if !types.Identical(results.At(1).Type(), types.Universe.Lookup("error").Type()) {
			return nil, fmt.Errorf("second return value must be error")
		}
		return results.At(0).Type(), nil
	default:
		return nil, fmt.Errorf("constructor must return (T) or (T, error)")
	}
}

// toResolvedType converts a go/types type into the generator's resolvedType.
// Shapes the generator cannot emit (slices, maps, generics, ...) keep their
// textual form in Raw, so code generation reports them as unsupported.
func toResolvedType(t types.Type) resolvedType {
	if pointer, ok := t.(*types.Pointer); ok {
		inner := toResolvedType(pointer.Elem())
		if inner.Raw == "" {
			inner.Pointer = true
			return inner
		}
		return resolvedType{Raw: t.String()}
	}
	switch typed := t.(type) {
	case *types.Basic:
		return resolvedType{Name: typed.Name()}
	case *types.Named:
		if typed.TypeArgs().Len() > 0 {
			return resolvedType{Raw: t.String()}
		}
		obj := typed.Obj()
		if obj.Pkg() == nil { // the predeclared error type
			return resolvedType{Name: obj.Name()}
		}
		return resolvedType{ImportPath: obj.Pkg().Path(), Name: obj.Name()}
	}
	return resolvedType{Raw: t.String()}
}

// typeLoader type-checks packages from source, resolving import paths
// relative to srcDir so module-aware lookups work from the app.
type typeLoader struct {
	importer types.ImporterFrom
	srcDir   string
}

// newTypeLoader returns a loader whose `go list` lookups run inside srcDir.
// The source importer reads build.Default, which has no directory of its own,
// so Dir is pointed at srcDir until the returned release func is called.
func newTypeLoader(srcDir string) (*typeLoader, func(), error) {
	imp, ok := importer.ForCompiler(token.NewFileSet(), "source", nil).(types.ImporterFrom)
	if !ok {
		return nil, nil, fmt.Errorf("source importer does not support relative imports")
	}
	previousDir := build.Default.Dir
	build.Default.Dir = srcDir
	release := func() { build.Default.Dir = previousDir }
	return &typeLoader{importer: imp, srcDir: srcDir}, release, nil
}

func (l *typeLoader) load(pkgPath string) (*types.Package, error) {
	pkg, err := l.importer.ImportFrom(pkgPath, l.srcDir, 0)
	if err != nil {
		return nil, fmt.Errorf("load package %s: %w", pkgPath, err)
	}
	return pkg, nil
}

func (l *typeLoader) interfaceType(pkgPath, name string) (*types.Interface, error) {
	pkg, err := l.load(pkgPath)
	if err != nil {
		return nil, err
	}
	obj, ok := pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok {
		return nil, fmt.Errorf("%s.%s is not a type", pkgPath, name)
	}
	iface, ok := obj.Type().Underlying().(*types.Interface)
	if !ok {
		return nil, fmt.Errorf("%s.%s is not an interface", pkgPath, name)
	}
	return iface, nil
}

func (l *typeLoader) constructorSignature(pkgPath, name string) (*types.Signature, error) {
	pkg, err := l.load(pkgPath)
	if err != nil {
		return nil, err
	}
	fn, ok := pkg.Scope().Lookup(name).(*types.Func)
	if !ok {
		return nil, fmt.Errorf("%s.%s is not a function", pkgPath, name)
	}
	return fn.Type().(*types.Signature), nil
}
