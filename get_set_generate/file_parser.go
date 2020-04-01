
package main

// This file contains the model construction by parsing source files.

import (
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"log"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	imports  = flag.String("imports", "", "(source mode) Comma-separated name=path pairs of explicit imports to use.")
)

// TODO: simplify error reporting
///Users/amanbansal/go/src/gitlab.com/machodev/go_get_set_generator/get_set_generate
func ParseFile(source string) (*FileInfo, error) {
	srcDir, err := filepath.Abs(filepath.Dir(source))
	if err != nil {
		return nil, fmt.Errorf("failed getting source directory: %v", err)
	}
	fmt.Println(srcDir)
	var packageImport string
	if p, err := build.ImportDir(srcDir, 0); err == nil {
		packageImport = p.ImportPath
	} // TODO: should we fail if this returns an error?

	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, source, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("failed parsing source file %v: %v", source, err)
	}

	p := &fileParser{
		fileSet:            fs,
		imports:            make(map[string]string),
		importedInterfaces: make(map[string]map[string]*ast.StructType),
		srcDir:             srcDir,
	}

	// Handle -imports.
	dotImports := make(map[string]bool)
	if *imports != "" {
		for _, kv := range strings.Split(*imports, ",") {
			eq := strings.Index(kv, "=")
			k, v := kv[:eq], kv[eq+1:]
			if k == "." {
				// TODO: Catch dupes?
				dotImports[v] = true
			} else {
				// TODO: Catch dupes?
				p.imports[k] = v
			}
		}
	}

	pkg, err := p.parseFile(packageImport, file)
	if err != nil {
		return nil, err
	}
	for path := range dotImports {
		pkg.DotImports = append(pkg.DotImports, path)
	}
	return pkg, nil
}

type fileParser struct {
	fileSet            *token.FileSet
	imports            map[string]string                        // package name => import path
	importedInterfaces map[string]map[string]*ast.StructType // package (or "") => name => interface

	auxFiles      []*ast.File

	srcDir string
}

func (p *fileParser) errorf(pos token.Pos, format string, args ...interface{}) error {
	ps := p.fileSet.Position(pos)
	format = "%s:%d:%d: " + format
	args = append([]interface{}{ps.Filename, ps.Line, ps.Column}, args...)
	return fmt.Errorf(format, args...)
}

// parseFile loads all file imports and auxiliary files import into the
// fileParser, parses all file interfaces and returns package model.
func (p *fileParser) parseFile(importPath string, file *ast.File) (*FileInfo, error) {
	allImports, dotImports := importsOfFile(file)
	// Don't stomp imports provided by -imports. Those should take precedence.
	for pkg, path := range allImports {
		if _, ok := p.imports[pkg]; !ok {
			p.imports[pkg] = path
		}
	}
	// Add imports from auxiliary files, which might be needed for embedded interfaces.
	// Don't stomp any other imports.
	for _, f := range p.auxFiles {
		auxImports, _ := importsOfFile(f)
		for pkg, path := range auxImports {
			if _, ok := p.imports[pkg]; !ok {
				p.imports[pkg] = path
			}
		}
	}

	var is []*Struct
	for ni := range iterStructs(file) {
		i, err := p.parseStruct(ni.name.String(), importPath, ni.it)
		if err != nil {
			return nil, err
		}
		is = append(is, i)
	}
	return &FileInfo{
		ImportPath: importPath,
		PackageName: "test",
		Name:        file.Name.String(),
		Structs:     is,
		DotImports:  dotImports,
	}, nil
}

// parsePackage loads package specified by path, parses it and populates
// corresponding imports and importedInterfaces into the fileParser.
func (p *fileParser) parsePackage(path string) error {
	var pkgs map[string]*ast.Package
	if imp, err := build.Import(path, p.srcDir, build.FindOnly); err != nil {
		return err
	} else if pkgs, err = parser.ParseDir(p.fileSet, imp.Dir, nil, 0); err != nil {
		return err
	}
	for _, pkg := range pkgs {
		file := ast.MergePackageFiles(pkg, ast.FilterFuncDuplicates|ast.FilterUnassociatedComments|ast.FilterImportDuplicates)
		if _, ok := p.importedInterfaces[path]; !ok {
			p.importedInterfaces[path] = make(map[string]*ast.StructType)
		}
		for ni := range iterStructs(file) {
			p.importedInterfaces[path][ni.name.Name] = ni.it
		}
		imports, _ := importsOfFile(file)
		for pkgName, pkgPath := range imports {
			if _, ok := p.imports[pkgName]; !ok {
				p.imports[pkgName] = pkgPath
			}
		}
	}
	return nil
}

func (p *fileParser) parseStruct(name, pkg string, it *ast.StructType) (*Struct, error) {
	intf := &Struct{Name: name}
	fields := make([]*Field, len(it.Fields.List))
	fmt.Println("Length of fields", len(it.Fields.List))
	for index, field := range it.Fields.List {
		parseType, err := p.parseType(pkg, field.Type)
		if err != nil {
			return nil, err
		}
		fields[index] = &Field{
			Name: field.Names[0].String(),
			Type: Parameter{
				Name: "",
				Type: parseType,
			},
		}
	}
	intf.Fields = fields
	return intf, nil
}

func (p *fileParser) parseType(pkg string, typ ast.Expr) (Type, error) {
	switch v := typ.(type) {
	case *ast.ArrayType:
		ln := -1
		if v.Len != nil {
			x, err := strconv.Atoi(v.Len.(*ast.BasicLit).Value)
			if err != nil {
				return nil, p.errorf(v.Len.Pos(), "bad array size: %v", err)
			}
			ln = x
		}
		t, err := p.parseType(pkg, v.Elt)
		if err != nil {
			return nil, err
		}
		return &ArrayType{Len: ln, Type: t}, nil
	case *ast.ChanType:
		t, err := p.parseType(pkg, v.Value)
		if err != nil {
			return nil, err
		}
		var dir ChanDir
		if v.Dir == ast.SEND {
			dir = SendDir
		}
		if v.Dir == ast.RECV {
			dir = RecvDir
		}
		return &ChanType{Dir: dir, Type: t}, nil
	case *ast.Ellipsis:
		// assume we're parsing a variadic argument
		return p.parseType(pkg, v.Elt)
	case *ast.Ident:
		if v.IsExported() {
			// `pkg` may be an aliased imported pkg
			// if so, patch the import w/ the fully qualified import
			maybeImportedPkg, ok := p.imports[pkg]
			if ok {
				pkg = maybeImportedPkg
			}
			// assume type in this package
			return &NamedType{Package: pkg, Type: v.Name}, nil
		} else {
			// assume predeclared type
			return PredeclaredType(v.Name), nil
		}
	case *ast.InterfaceType:
		if v.Methods != nil && len(v.Methods.List) > 0 {
			return nil, p.errorf(v.Pos(), "can't handle non-empty unnamed interface types")
		}
		return PredeclaredType("interface{}"), nil
	case *ast.MapType:
		key, err := p.parseType(pkg, v.Key)
		if err != nil {
			return nil, err
		}
		value, err := p.parseType(pkg, v.Value)
		if err != nil {
			return nil, err
		}
		return &MapType{Key: key, Value: value}, nil
	case *ast.SelectorExpr:
		pkgName := v.X.(*ast.Ident).String()
		pkg, ok := p.imports[pkgName]
		if !ok {
			return nil, p.errorf(v.Pos(), "unknown package %q", pkgName)
		}
		return &NamedType{Package: pkg, Type: v.Sel.String()}, nil
	case *ast.StarExpr:
		t, err := p.parseType(pkg, v.X)
		if err != nil {
			return nil, err
		}
		return &PointerType{Type: t}, nil
	case *ast.StructType:
		if v.Fields != nil && len(v.Fields.List) > 0 {
			return nil, p.errorf(v.Pos(), "can't handle non-empty unnamed struct types")
		}
		return PredeclaredType("struct{}"), nil
	}

	return nil, fmt.Errorf("don't know how to parse type %T", typ)
}

// importsOfFile returns a map of package name to import path
// of the imports in file.
func importsOfFile(file *ast.File) (normalImports map[string]string, dotImports []string) {
	normalImports = make(map[string]string)
	dotImports = make([]string, 0)
	for _, is := range file.Imports {
		var pkgName string
		importPath := is.Path.Value[1 : len(is.Path.Value)-1] // remove quotes

		if is.Name != nil {
			// Named imports are always certain.
			if is.Name.Name == "_" {
				continue
			}
			pkgName = is.Name.Name
		} else {
			pkg, err := build.Import(importPath, "", 0)
			if err != nil {
				// Fallback to import path suffix. Note that this is uncertain.
				_, last := path.Split(importPath)
				// If the last path component has dots, the first dot-delimited
				// field is used as the name.
				pkgName = strings.SplitN(last, ".", 2)[0]
			} else {
				pkgName = pkg.Name
			}
		}

		if pkgName == "." {
			dotImports = append(dotImports, importPath)
		} else {

			if _, ok := normalImports[pkgName]; ok {
				log.Fatalf("imported package collision: %q imported twice", pkgName)
			}
			normalImports[pkgName] = importPath
		}
	}
	return
}

type namedInterface struct {
	name *ast.Ident
	it   *ast.StructType
}

// Create an iterator over all interfaces in file.
func iterStructs(file *ast.File) <-chan namedInterface {
	ch := make(chan namedInterface)
	go func() {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				it, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}

				ch <- namedInterface{ts.Name, it}
			}
		}
		close(ch)
	}()
	return ch
}

// isVariadic returns whether the function is variadic.
func isVariadic(f *ast.FuncType) bool {
	nargs := len(f.Params.List)
	if nargs == 0 {
		return false
	}
	_, ok := f.Params.List[nargs-1].Type.(*ast.Ellipsis)
	return ok
}
