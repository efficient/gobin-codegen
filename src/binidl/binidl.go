package binidl

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	//	"go/scanner"
	//"go/printer"
	"reflect"
)

type Binidl struct {
	ast  *ast.File
	fset *token.FileSet
}

func NewBinidl(filename string) *Binidl {
	fset := token.NewFileSet()
	ast, err := parser.ParseFile(fset, filename, nil, 0) // scanner.InsertSemis)
	if err != nil {
		fmt.Println("Error parsing", filename, ":", err)
		return nil
	}
	return &Binidl{ast, fset}
}

func (bf *Binidl) Visit(n ast.Node) ast.Visitor {
	fmt.Println("Visiting ", n)
	return bf
}

func b(n int) {
	fmt.Printf("\tbs = b[:%d]\n", n)
}
func bs(x, y, z string) {
	fmt.Printf("\t%s(bs, %s(%s))\n", x, y, z)
}
func wbs() {
	fmt.Println("\tw.Write(bs)")
}

func marshalField(fname, tname, pred string) {
	f := pred + "." + fname
	switch tname {
	case "int", "int64", "uint64":
		b(8)
		bs("binary.LittleEndian.PutUint64", "uint64", f)
		wbs()
	default:
		fmt.Printf("\tMarshal%s(&%s, w)\n", tname, f)
	}
}

func marshalContents(ts *ast.TypeSpec, pred string) {
	//fmt.Println("Marshaling ", ts, " with ", pred)
	switch ts.Type.(type) {
	case *ast.StructType:
		st := ts.Type.(*ast.StructType)
		//fmt.Println("Hey, a struct.  Recurse on its contents.")
		for _, f := range st.Fields.List {
			fname := f.Names[0].Name
			t := f.Type.(*ast.Ident)
			//fmt.Println("obj: ", t.Obj)
			//fmt.Println("t: ", t)
			//fmt.Println("Field ", fname, " (", f, ") type ", reflect.TypeOf(f.Type))
			marshalField(fname, t.Name, pred)
		}
	default:
		panic("Don't know how to handle this type yet")
	}
}

func structmap(n interface{}) {
	decl, ok := n.(*ast.GenDecl)
	if !ok {
		return
	}
	//fmt.Println("Stmt: ", decl, " is ", reflect.TypeOf(decl))
	if decl.Tok != token.TYPE {
		return
	}
	//fmt.Println("Got a type!")
	ts := decl.Specs[0].(*ast.TypeSpec)
	typeName := ts.Name.Name
	//fmt.Println("ts: ", typeName)
	fmt.Printf("func Marshal%s(t *%s, w io.Writer) {\n", typeName, typeName)
	fmt.Println("\tvar b [8]byte")
	fmt.Println("\tbs := b[:8]")
	//fmt.Printf("tstype: ", reflect.TypeOf(ts.Type))
	marshalContents(ts, "t")
	fmt.Println("}\n")
	return
}

func (bf *Binidl) PrintGo() {
	walk(bf.ast, structmap)
}

func let_me_keep_reflect_loaded_please() {
	fmt.Println(reflect.TypeOf(0))
}
