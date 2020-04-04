package models

import "fmt"

type Type interface {
	String(pm map[string]string, currentPackage string) string
	addImports(im map[string]bool)
}

// ArrayType is an array or slice type.
type ArrayType struct {
	Len  int // -1 for slices, >= 0 for arrays
	Type Type
}

func (at *ArrayType) String(pm map[string]string, currentPackage string) string {
	s := "[]"
	if at.Len > -1 {
		s = fmt.Sprintf("[%d]", at.Len)
	}
	return s + at.Type.String(pm, currentPackage)
}

func (at *ArrayType) addImports(im map[string]bool) { at.Type.addImports(im) }

// ChanType is a channel type.
type ChanType struct {
	Dir  ChanDir // 0, 1 or 2
	Type Type
}

func (ct *ChanType) String(pm map[string]string, currentPackage string) string {
	s := ct.Type.String(pm, currentPackage)
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

func (mt *MapType) String(pm map[string]string, currentPackage string) string {
	return "map[" + mt.Key.String(pm, currentPackage) + "]" + mt.Value.String(pm, currentPackage)
}

func (mt *MapType) addImports(im map[string]bool) {
	mt.Key.addImports(im)
	mt.Value.addImports(im)
}

// NamedType is an exported type in a package.
type NamedType struct {
	ImportPath string // may be empty
	Type       string // TODO: should this be typed Type?
}

func (nt *NamedType) String(pm map[string]string, currentPackage string) string {
	prefix := pm[nt.ImportPath]
	if nt.ImportPath == currentPackage {
		return nt.Type
	} else if prefix != "" {
		return prefix + "." + nt.Type
	} else {
		return nt.Type
	}
}
func (nt *NamedType) addImports(im map[string]bool) {
	if nt.ImportPath != "" {
		im[nt.ImportPath] = true
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

func (pt PredeclaredType) String(pm map[string]string, currentPackage string) string {
	return string(pt)
}
func (pt PredeclaredType) addImports(im map[string]bool) {}
