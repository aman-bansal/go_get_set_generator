package main

import (
	"flag"
	"fmt"
	"github.com/aman-bansal/go_get_set_generator/get_set_generate/internal"
	"github.com/aman-bansal/go_get_set_generator/get_set_generate/internal/models"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	sourceFile = flag.String("source", "", "(source mode) Input Go source file; enables source mode.")
)

func main() {
	flag.Parse()
	fmt.Println("starting generate")
	if *sourceFile == "" {
		log.Fatal("Source file is empty")
	}

	var fileInfo *models.FileInfo
	var err error
	fileInfo, err = internal.ParseFile(*sourceFile)
	if err != nil {
		log.Fatalf("Loading input failed: %v", err)
	}

	//create destination file
	fileName, _ := filepath.Abs(*sourceFile)
	f, err := os.Create(strings.TrimSuffix(fileName, filepath.Ext(fileName)) + "_getter_setter.go")
	if err != nil {
		log.Fatalf("Failed opening destination file: %v", err)
	}

	defer func() { _ = f.Close() }()
	g := new(internal.Generator)
	if err := g.Generate(fileInfo); err != nil {
		log.Fatalf("Failed generating getter setter: %v", err)
	}

	if _, err := f.Write(g.Output()); err != nil {
		log.Fatalf("Failed writing to destination: %v", err)
	}
	fmt.Println("code generation done")
}
