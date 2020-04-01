package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"go/token"
	"io"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"unicode"
)

var (
	source_file          = flag.String("source", "", "(source mode) Input Go source file; enables source mode.")
)

type FileInfo struct {
	ImportPath  string
	PackageName string
	Name        string
	Structs     []*Struct
	DotImports  []string
}

// Imports returns the imports needed by the Package as a set of import paths.
func (pkg *FileInfo) Imports() map[string]bool {
	im := make(map[string]bool)
	for _, strct := range pkg.Structs {
		strct.addImports(im)
	}
	return im
}

type Struct struct {
	Name string
	Fields []*Field
}

func (intf *Struct) Print(w io.Writer) {
	fmt.Fprintf(w, "interface %s\n", intf.Name)
	for _, m := range intf.Fields {
		m.Print(w)
	}
}

func (intf *Struct) addImports(im map[string]bool) {
	for _, m := range intf.Fields {
		m.addImports(im)
	}
}

type Field struct {
	Name string
	Type Parameter

}

type Parameter struct {
	Name string // may be empty
	Type Type
}

func (p *Parameter) Print(w io.Writer) {
	n := p.Name
	if n == "" {
		n = `""`
	}
	fmt.Fprintf(w, "    - %v: %v\n", n, p.Type.String(nil, ""))
}

func (m *Field) Print(w io.Writer) {
	fmt.Fprintf(w, "  - method %s\n", m.Name)
	fmt.Fprintf(w, "    in:\n")
	m.Type.Print(w)
}

func (m *Field) addImports(im map[string]bool) {
	m.Type.Type.addImports(im)
}

type Type interface {
	String(pm map[string]string, currentPackage string) string
	addImports(im map[string]bool)
}

func init() {
	gob.Register(&ArrayType{})
	gob.Register(&ChanType{})
	gob.Register(&MapType{})
	gob.Register(&NamedType{})
	gob.Register(&PointerType{})
	gob.RegisterName(".PredeclaredType", PredeclaredType(""))
}

// ArrayType is an array or slice type.
type ArrayType struct {
	Len  int // -1 for slices, >= 0 for arrays
	Type Type
}

func (at *ArrayType) String(pm map[string]string, currentPackge string) string {
	s := "[]"
	if at.Len > -1 {
		s = fmt.Sprintf("[%d]", at.Len)
	}
	return s + at.Type.String(pm, currentPackge)
}

func (at *ArrayType) addImports(im map[string]bool) { at.Type.addImports(im) }

// ChanType is a channel type.
type ChanType struct {
	Dir  ChanDir // 0, 1 or 2
	Type Type
}

func (ct *ChanType) String(pm map[string]string, currentPackge string) string {
	s := ct.Type.String(pm, currentPackge)
	if ct.Dir == RecvDir {
		return "<-chan " + s
	}
	if ct.Dir == SendDir {
		return "chan<- " + s
	}
	return "chan " + s
}

func (ct *ChanType) addImports(im map[string]bool) { ct.Type.addImports(im) }

type ChanDir int

const (
	RecvDir ChanDir = 1
	SendDir ChanDir = 2
)

// MapType is a map type.
type MapType struct {
	Key, Value Type
}

func (mt *MapType) String(pm map[string]string, currentPackge string) string {
	return "map[" + mt.Key.String(pm, currentPackge) + "]" + mt.Value.String(pm, currentPackge)
}

func (mt *MapType) addImports(im map[string]bool) {
	mt.Key.addImports(im)
	mt.Value.addImports(im)
}

// NamedType is an exported type in a package.
type NamedType struct {
	Package string // may be empty
	Type    string // TODO: should this be typed Type?
}

func (nt *NamedType) String(pm map[string]string, currentPackage string) string {
	fmt.Println("PackageName is ", nt.Package)
	fmt.Println("package mape is ", pm)
	prefix := pm[nt.Package]
	if nt.Package == currentPackage {
		return nt.Type
	} else if prefix != "" {
		return prefix + "." + nt.Type
	} else {
		return nt.Type
	}
}
func (nt *NamedType) addImports(im map[string]bool) {
	if nt.Package != "" {
		im[nt.Package] = true
	}
}

// PointerType is a pointer to another type.
type PointerType struct {
	Type Type
}

func (pt *PointerType) String(pm map[string]string, currentPackage string) string {
	return "*" + pt.Type.String(pm, currentPackage)
}
func (pt *PointerType) addImports(im map[string]bool) { pt.Type.addImports(im) }

// PredeclaredType is a predeclared type such as "int".
type PredeclaredType string

func (pt PredeclaredType) String(pm map[string]string, currentPackage string) string { return string(pt) }
func (pt PredeclaredType) addImports(im map[string]bool)                          {}

type generator struct {
	buf        bytes.Buffer
	indent     string
	filename   string // may be empty
	srcPackage string // may be empty

	packageMap map[string]string // map from import path to package name
}

func (g *generator) p(format string, args ...interface{}) {
	fmt.Fprintf(&g.buf, g.indent+format+"\n", args...)
}

func (g *generator) in() {
	g.indent += "\t"
}

func (g *generator) out() {
	if len(g.indent) > 0 {
		g.indent = g.indent[0 : len(g.indent)-1]
	}
}

func main() {
	fmt.Println("starting generate")
	*source_file = "sample.go"
	if *source_file == "" {
		log.Fatal("Source file is empty")
	}

	fmt.Println(*source_file)
	var fileInfo *FileInfo
	var err error
	fileInfo, err = ParseFile(*source_file)
	if err != nil {
		log.Fatalf("Loading input failed: %v", err)
	}

	f, err := os.Create("getter_setter.go")
	if err != nil {
		log.Fatalf("Failed opening destination file: %v", err)
	}
	defer f.Close()
	dst := f
	packageName := fileInfo.PackageName
	g := new(generator)
	g.filename = "getter_setter.go"
	if err := g.Generate(fileInfo, packageName); err != nil {
		log.Fatalf("Failed generating getter setter: %v", err)
	}
	if _, err := dst.Write(g.Output()); err != nil {
		log.Fatalf("Failed writing to destination: %v", err)
	}
}

func (g *generator) Generate(pkg *FileInfo, pkgName string) error {
	g.p("// Code generated by getter and setter. DO NOT EDIT.")
	if g.filename != "" {
		g.p("// Source: %v", g.filename)
	} else {
		g.p("// Source: %v", g.srcPackage)
	}
	g.p("")

	// Get all required imports, and generate unique names for them all.
	im := pkg.Imports()

	// Sort keys to make import alias generation predictable
	sorted_paths := make([]string, len(im), len(im))
	x := 0
	for pth := range im {
		sorted_paths[x] = pth
		x++
	}
	sort.Strings(sorted_paths)

	g.packageMap = make(map[string]string, len(im))
	localNames := make(map[string]bool, len(im))
	for _, pth := range sorted_paths {
		base := sanitize(path.Base(pth))

		// Local names for an imported package can usually be the basename of the import path.
		// A couple of situations don't permit that, such as duplicate local names
		// (e.g. importing "html/template" and "text/template"), or where the basename is
		// a keyword (e.g. "foo/case").
		// try base0, base1, ...
		pkgName := base
		i := 0
		for localNames[pkgName] || token.Lookup(pkgName).IsKeyword() {
			pkgName = base + strconv.Itoa(i)
			i++
		}

		g.packageMap[pth] = pkgName
		localNames[pkgName] = true
	}

	g.p("package %v", pkgName)
	g.p("")
	g.p("import (")
	g.in()
	for path, pkgName := range g.packageMap {
		if path != pkg.ImportPath {
			g.p("%v %q", pkgName, path)
		}
	}
	for _, path := range pkg.DotImports {
		g.p(". %q", path)
	}
	g.out()
	g.p(")")

	for _, strcts := range pkg.Structs {
		if err := g.GenerateGetterAndSetters(strcts, pkg.ImportPath); err != nil {
			return err
		}
	}

	return nil
}

func (g *generator) GenerateGetterAndSetters(strct *Struct, currentPackage string) error {
	g.p("")
	g.p("// This is a getter and setter of %v ", strct.Name)

	bytes, _ := json.Marshal(strct)
	fmt.Println(string(bytes))
	for _, field := range strct.Fields {
		g.p("func (%v *%v) Get%v() %v {", strct.Name, strct.Name, field.Name, field.Type.Type.String(g.packageMap, currentPackage))
		g.in()
		g.p("return %v.%v", strct.Name, field.Name)
		g.out()
		g.p("}")
		g.p("")


		g.p("func (%v *%v) Set%v(val %v) {", strct.Name, strct.Name, field.Name, field.Type.Type.String(g.packageMap, currentPackage))
		g.in()
		g.p("%v.%v = val", strct.Name, field.Name)
		g.out()
		g.p("}")
		g.p("")
	}

	return nil
}

func (g *generator) Output() []byte {
	src, err := format.Source(g.buf.Bytes())
	if err != nil {
		log.Fatalf("Failed to format generated source code: %s\n%s", err, g.buf.String())
	}
	return src
}


// sanitize cleans up a string to make a suitable package name.
func sanitize(s string) string {
	t := ""
	for _, r := range s {
		if t == "" {
			if unicode.IsLetter(r) || r == '_' {
				t += string(r)
				continue
			}
		} else {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
				t += string(r)
				continue
			}
		}
		t += "_"
	}
	if t == "_" {
		t = "x"
	}
	return t
}