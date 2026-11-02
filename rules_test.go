package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

func parseAppSource(t *testing.T) []*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, file := range pkgs["main"].Files {
		files = append(files, file)
	}
	return files
}

func receiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	typ := fn.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if ident, ok := typ.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func TestCoreHasNoExportedMethods(t *testing.T) {
	for _, file := range parseAppSource(t) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && receiverTypeName(fn) == "core" && fn.Name.IsExported() {
				t.Errorf("core.%s is exported: every service embeds the core, so Wails would bind it once per service", fn.Name.Name)
			}
		}
	}
}

type eventSource struct {
	consts     map[string]string
	registered map[string]ast.Expr
	types      map[string]ast.Expr
	emitted    []*ast.CallExpr
}

func collectEvents(t *testing.T) eventSource {
	src := eventSource{consts: map[string]string{}, registered: map[string]ast.Expr{}, types: map[string]ast.Expr{}}
	for _, file := range parseAppSource(t) {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				switch spec := spec.(type) {
				case *ast.ValueSpec:
					if gen.Tok != token.CONST {
						continue
					}
					for i, name := range spec.Names {
						if i >= len(spec.Values) {
							continue
						}
						if lit, ok := spec.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							value, _ := strconv.Unquote(lit.Value)
							src.consts[name.Name] = value
						}
					}
				case *ast.TypeSpec:
					src.types[spec.Name.Name] = spec.Type
				}
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.IndexExpr:
				if sel, ok := fun.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "RegisterEvent" {
					if name, ok := call.Args[0].(*ast.Ident); ok {
						src.registered[name.Name] = fun.Index
					} else {
						t.Errorf("RegisterEvent at %v takes %T, want a named constant", call.Pos(), call.Args[0])
					}
				}
			case *ast.SelectorExpr:
				switch fun.Sel.Name {
				case "Emit", "emit", "emitOperationEvent":
					src.emitted = append(src.emitted, call)
				}
			}
			return true
		})
	}
	return src
}

func TestEveryEmittedEventIsARegisteredConstant(t *testing.T) {
	src := collectEvents(t)
	for _, call := range src.emitted {
		switch name := call.Args[0].(type) {
		case *ast.BasicLit:
			t.Errorf("an event is emitted by the literal name %s: name it with a constant next to its RegisterEvent", name.Value)
		case *ast.Ident:
			if _, isConst := src.consts[name.Name]; isConst {
				if _, ok := src.registered[name.Name]; !ok {
					t.Errorf("event %s (%q) is emitted but never registered, so its payload reaches the frontend untyped", name.Name, src.consts[name.Name])
				}
			}
		}
	}
}

// op:done ends every operation and transfers:finished ends each job of the transfer queue, which runs outside the operation runner.
var completionEvents = map[string]bool{"op:done": true, "transfers:finished": true}

func TestNoNewCompletionEvents(t *testing.T) {
	src := collectEvents(t)
	for constName := range src.registered {
		value := src.consts[constName]
		for _, suffix := range []string{":done", ":complete", ":completed", ":finished"} {
			if strings.HasSuffix(value, suffix) && !completionEvents[value] {
				t.Errorf("event %q reports completion on its own: run the flow through runOperation and return its result in op:done", value)
			}
		}
	}
}

func TestRegisteredPayloadsHaveNoAny(t *testing.T) {
	src := collectEvents(t)
	seen := map[string]bool{}
	var check func(path string, expr ast.Expr)
	check = func(path string, expr ast.Expr) {
		switch expr := expr.(type) {
		case *ast.Ident:
			if expr.Name == "any" {
				t.Errorf("%s is any, which generates as any and leaves the frontend casting", path)
				return
			}
			if declared, ok := src.types[expr.Name]; ok && !seen[expr.Name] {
				seen[expr.Name] = true
				check(expr.Name, declared)
			}
		case *ast.InterfaceType:
			t.Errorf("%s is an interface, which generates as any and leaves the frontend casting", path)
		case *ast.StarExpr:
			check(path, expr.X)
		case *ast.ArrayType:
			check(path, expr.Elt)
		case *ast.MapType:
			check(path, expr.Value)
		case *ast.StructType:
			for _, field := range expr.Fields.List {
				name := "(embedded)"
				if len(field.Names) > 0 {
					name = field.Names[0].Name
				}
				check(path+"."+name, field.Type)
			}
		}
	}
	for constName, payload := range src.registered {
		check(src.consts[constName], payload)
	}
}
