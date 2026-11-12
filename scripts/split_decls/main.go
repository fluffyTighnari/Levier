// Middleware — move top-level declarations between files in one package.
//
// Usage:
//
//	go run ./scripts/split_decls.go -src internal/x/a.go -map /tmp/map.json
//
// The map is {"declKey": "target.go"}. Unmapped declarations stay in src.
// Every target file receives the source import block; the build step prunes
// unused imports afterwards.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type item struct {
	key   string
	start int
	end   int
}

func main() {
	src := flag.String("src", "", "source Go file")
	mapPath := flag.String("map", "", "JSON mapping declKey -> target file")
	flag.Parse()
	if *src == "" || *mapPath == "" {
		fmt.Fprintln(os.Stderr, "usage: split_decls -src file.go -map mapping.json")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*mapPath)
	if err != nil {
		panic(err)
	}
	mapping := map[string]string{}
	if err := json.Unmarshal(raw, &mapping); err != nil {
		panic(err)
	}
	data, err := os.ReadFile(*src)
	if err != nil {
		panic(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, *src, data, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	offset := func(pos token.Pos) int { return fset.Position(pos).Offset }

	importText := ""
	var items []item
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			importText = string(data[offset(gen.Pos()):offset(gen.End())])
			continue
		}
		key := declKey(decl)
		start := offset(decl.Pos())
		end := offset(decl.End())
		if doc := declDoc(decl); doc != nil {
			start = offset(doc.Pos())
		}
		items = append(items, item{key: key, start: start, end: end})
	}

	groups := map[string][]item{}
	for _, entry := range items {
		target := mapping[entry.key]
		if target == "" {
			target = filepath.Base(*src)
		}
		groups[target] = append(groups[target], entry)
	}

	dir := filepath.Dir(*src)
	for target, entries := range groups {
		sort.Slice(entries, func(i, j int) bool { return entries[i].start < entries[j].start })
		var builder strings.Builder
		builder.WriteString("// Middleware — " + file.Name.Name + "\n//\n")
		builder.WriteString("// Split from " + filepath.Base(*src) + " (P8R.3).\n")
		builder.WriteString("// Copyright 2026 The Middleware Authors.\n")
		builder.WriteString("package " + file.Name.Name + "\n\n")
		if importText != "" {
			builder.WriteString(importText + "\n\n")
		}
		for _, entry := range entries {
			builder.Write(data[entry.start:entry.end])
			builder.WriteString("\n\n")
		}
		path := filepath.Join(dir, target)
		if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
			panic(err)
		}
		fmt.Printf("wrote %s (%d decls)\n", path, len(entries))
	}
}

func declKey(decl ast.Decl) string {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		if typed.Recv != nil && len(typed.Recv.List) > 0 {
			return recvName(typed.Recv.List[0].Type) + "." + typed.Name.Name
		}
		return typed.Name.Name
	case *ast.GenDecl:
		for _, spec := range typed.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				return s.Name.Name
			case *ast.ValueSpec:
				if len(s.Names) > 0 {
					return s.Names[0].Name
				}
			}
		}
	}
	return ""
}

func recvName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.StarExpr:
		return recvName(typed.X)
	case *ast.Ident:
		return typed.Name
	case *ast.IndexExpr:
		return recvName(typed.X)
	}
	return ""
}

func declDoc(decl ast.Decl) *ast.CommentGroup {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		return typed.Doc
	case *ast.GenDecl:
		return typed.Doc
	}
	return nil
}
