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

func r(n int) {
	fmt.Printf("\tReading %d\n", n)
}

func unmarshalField(fname, tname, pred string) {
	f := pred + "." + fname
	switch tname {
	case "int", "int64", "uint64":
		r(8)
	case "int32", "uint32":
		r(4)
	case "int16", "uint16":
		r(2)
	case "int8", "uint8":
		r(1)
	default:
		fmt.Printf("\tUnmarshal%s(&%s, w)\n", tname, f)
	}
	fmt.Println("Hi, I don't do this yet: ", f)
}

func unmarshalContents(ts *ast.TypeSpec, pred string) {
	fmt.Println("\tNot implemented yet")
}

func marshalField(fname, tname, pred string) {
	f := pred + "." + fname
	switch tname {
	case "int", "int64", "uint64":
		b(8)
		bs("binary.LittleEndian.PutUint64", "uint64", f)
		wbs()
	case "int32", "uint32":
		b(4)
		bs("binary.LittleEndian.PutUint32", "uint32", f)
		wbs()
	case "int16", "uint16":
		b(2)
		bs("binary.LittleEndian.PutUint16", "uint16", f)
		wbs()
	case "int8", "uint8":
		b(1)
		fmt.Printf("\tb[0] = byte(%s)\n", f)
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
			switch f.Type.(type) {
			case *ast.Ident:
				t := f.Type.(*ast.Ident)
				marshalField(fname, t.Name, pred)
			case *ast.SelectorExpr:
				se := f.Type.(*ast.SelectorExpr)
				fmt.Printf("\t%s.Marshal%s(&%s, w)\n",
					se.X, se.Sel.Name, pred+"."+fname)
				
			}

			//fmt.Println("obj: ", t.Obj)
			//fmt.Println("t: ", t)
			//fmt.Println("Field ", fname, " (", f, ") type ", reflect.TypeOf(f.Type))
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

	fmt.Printf("func Unmarshal%s(t *%s, r io.Reader) {\n", typeName, typeName)
	fmt.Println("\tvar b [8]byte")
	fmt.Println("\tvar bs []byte")
	unmarshalContents(ts, "t")
	fmt.Println("}\n")
	return
}

func (bf *Binidl) PrintGo() {
	walk(bf.ast, structmap)
}

func let_me_keep_reflect_loaded_please() {
	fmt.Println(reflect.TypeOf(0))
}
