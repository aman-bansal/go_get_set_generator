package models

type FileInfo struct {
	ImportPath  string
	PackageName string
	Name        string
	Structs     []*Struct
	DotImports  []string
}

// Imports returns the imports needed by the ImportPath as a set of import paths.
func (file *FileInfo) Imports() map[string]bool {
	im := make(map[string]bool)
	for _, strct := range file.Structs {
		strct.addImports(im)
	}
	return im
}

type Struct struct {
	Name   string
	Fields []*Field
}

func (strct *Struct) addImports(im map[string]bool) {
	for _, m := range strct.Fields {
		m.addImports(im)
	}
}

type Field struct {
	Name string
	Type Type
}

func (field *Field) addImports(im map[string]bool) {
	field.Type.addImports(im)
}
