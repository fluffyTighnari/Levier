// Levier — report long Go functions using go/ast (P8R.1).
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type entry struct {
	size int
	pos  string
	name string
}

func main() {
	limit := 80
	if len(os.Args) > 1 {
		if value, err := strconv.Atoi(os.Args[1]); err == nil {
			limit = value
		}
	}
	var entries []entry
	for _, root := range []string{"internal", "cmd"} {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil
			}
			ast.Inspect(file, func(node ast.Node) bool {
				fn, ok := node.(*ast.FuncDecl)
				if !ok {
					return true
				}
				start := fset.Position(fn.Pos()).Line
				end := fset.Position(fn.End()).Line
				size := end - start + 1
				if size > limit {
					entries = append(entries, entry{
						size: size,
						pos:  fmt.Sprintf("%s:%d", path, start),
						name: fn.Name.Name,
					})
				}
				return true
			})
			return nil
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].size != entries[j].size {
			return entries[i].size > entries[j].size
		}
		return entries[i].pos < entries[j].pos
	})
	for _, item := range entries {
		fmt.Printf("%4d lines  %s  %s\n", item.size, item.pos, item.name)
	}
	if len(entries) == 0 {
		fmt.Println("(none)")
	}
}
