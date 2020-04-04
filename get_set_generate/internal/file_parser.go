package internal

// This file contains the model construction by parsing source files.

import (
	"fmt"
	"github.com/aman-bansal/go_get_set_generator/get_set_generate/internal/models"
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

func ParseFile(source string) (*models.FileInfo, error) {
	srcDir, err := filepath.Abs(filepath.Dir(source))
	if err != nil {
		return nil, fmt.Errorf("failed getting source directory: %v", err)
	}
	var packageImport string
	var packageName string
	if p, err := build.ImportDir(srcDir, 0); err == nil {
		packageImport = p.ImportPath
		packageName = p.Name
	} else {
		return nil, err
	}

	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, source, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("failed parsing source file %v: %v", source, err)
	}

	p := &fileParser{
		packageImportPath:       packageImport,
		currentPackage:          packageName,
		fileSet:                 fs,
		packageNameVsImportPath: make(map[string]string),
		structs:                 make(map[string]map[string]*ast.StructType),
		srcDir:                  srcDir,
	}

	return p.parseFile(packageImport, file)
}

type fileParser struct {
	packageImportPath       string
	currentPackage          string
	fileSet                 *token.FileSet
	packageNameVsImportPath map[string]string                     // package naem => import path
	structs                 map[string]map[string]*ast.StructType // package (or "") => structName => interface
	srcDir                  string
}

// parseFile loads all file packageNameVsImportPath and auxiliary files import into the
// fileParser, parses all file interfaces and returns package model.
func (p *fileParser) parseFile(packageImportPath string, file *ast.File) (*models.FileInfo, error) {
	allImports, dotImports := importsOfFile(file)
	p.packageNameVsImportPath = allImports

	var structs []*models.Struct
	for _, s := range iterStructs(file) {
		i, err := p.parseStruct(packageImportPath, s.structName, s.structType)
		if err != nil {
			return nil, err
		}
		structs = append(structs, i)
	}

	return &models.FileInfo{
		ImportPath:  packageImportPath,
		PackageName: p.currentPackage,
		Name:        file.Name.String(),
		Structs:     structs,
		DotImports:  dotImports,
	}, nil
}

func (p *fileParser) parseStruct(packageImportPath string, structName string, it *ast.StructType) (*models.Struct, error) {
	structInfo := &models.Struct{Name: structName}
	fields := make([]*models.Field, len(it.Fields.List))
	for index, field := range it.Fields.List {
		parseType, err := p.parseType(field.Type)
		if err != nil {
			return nil, err
		}
		fields[index] = &models.Field{
			Name: field.Names[0].String(),
			Type: parseType,
		}
	}
	structInfo.Fields = fields
	return structInfo, nil
}

func (p *fileParser) parseType(typ ast.Expr) (models.Type, error) {
	switch v := typ.(type) {
	case *ast.ArrayType:
		ln := -1
		if v.Len != nil {
			x, err := strconv.Atoi(v.Len.(*ast.BasicLit).Value)
			if err != nil {
				return nil, fmt.Errorf("bad array size with error %v", err)
			}
			ln = x
		}
		t, err := p.parseType(v.Elt)
		if err != nil {
			return nil, err
		}
		return &models.ArrayType{Len: ln, Type: t}, nil
	case *ast.ChanType:
		t, err := p.parseType(v.Value)
		if err != nil {
			return nil, err
		}
		var dir models.ChanDir
		if v.Dir == ast.SEND {
			dir = models.SendDir
		}
		if v.Dir == ast.RECV {
			dir = models.RecvDir
		}
		return &models.ChanType{Dir: dir, Type: t}, nil
	case *ast.Ident:
		if v.IsExported() {
			// `pkg` may be an aliased imported pkg
			// if so, patch the import w/ the fully qualified import
			maybeImportedPkg, ok := p.packageNameVsImportPath[p.currentPackage]
			if !ok {
				return &models.NamedType{ImportPath: p.packageImportPath, Type: v.Name}, nil
			}
			return &models.NamedType{ImportPath: maybeImportedPkg, Type: v.Name}, nil
		} else {
			// assume predeclared type
			return models.PredeclaredType(v.Name), nil
		}
	case *ast.InterfaceType:
		if v.Methods != nil && len(v.Methods.List) > 0 {
			return nil, fmt.Errorf("can't handle non-empty unnamed interface types")
		}
		return models.PredeclaredType("interface{}"), nil
	case *ast.MapType:
		key, err := p.parseType(v.Key)
		if err != nil {
			return nil, err
		}
		value, err := p.parseType(v.Value)
		if err != nil {
			return nil, err
		}
		return &models.MapType{Key: key, Value: value}, nil
	case *ast.SelectorExpr:
		pkgName := v.X.(*ast.Ident).String()
		pkg, ok := p.packageNameVsImportPath[pkgName]
		if !ok {
			return nil, fmt.Errorf("unknown package")
		}
		return &models.NamedType{ImportPath: pkg, Type: v.Sel.String()}, nil
	case *ast.StarExpr:
		t, err := p.parseType(v.X)
		if err != nil {
			return nil, err
		}
		return &models.PointerType{Type: t}, nil
	case *ast.StructType:
		if v.Fields != nil && len(v.Fields.List) > 0 {
			return nil, fmt.Errorf("can't handle non-empty unnamed struct types")
		}
		return models.PredeclaredType("struct{}"), nil
	}

	return nil, fmt.Errorf("don't know how to parse type %T", typ)
}

// importsOfFile returns a map of package structName to import path
// of the packageNameVsImportPath in file.
func importsOfFile(file *ast.File) (normalImports map[string]string, dotImports []string) {
	normalImports = make(map[string]string)
	dotImports = make([]string, 0)
	for _, is := range file.Imports {
		var pkgName string
		importPath := is.Path.Value[1 : len(is.Path.Value)-1] // remove quotes
		if is.Name != nil {
			// Named packageNameVsImportPath are always certain.
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
				// field is used as the structName.
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

type namedStruct struct {
	structName string
	structType *ast.StructType
}

func iterStructs(file *ast.File) []namedStruct {
	structs := make([]namedStruct, 0)
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

			structs = append(structs, namedStruct{ts.Name.String(), it})
		}
	}

	return structs
}
