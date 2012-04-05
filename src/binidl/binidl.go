package binidl

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	//	"go/scanner"
	//"go/printer"
	"reflect"
	"strconv"
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

var cur_b_marshal_state int = 0

func resetb() {
	cur_b_marshal_state = 8
}

func b(n int) {
	if (n != cur_b_marshal_state) {
		fmt.Printf("\tbs = b[:%d]\n", n)
	}
	cur_b_marshal_state = n
}
func bs(fname, tname, orderconvert string) {
	fmt.Printf("\tbinary.LittleEndian.Put%s(bs, %s(%s))\n", orderconvert, tname, fname)
}
func wbs() {
	fmt.Println("\tw.Write(bs)")
}

func r(n int) {
	b(n)
	fmt.Println("\tif _, err := io.ReadFull(r, bs); err != nil {")
	fmt.Println("\t\treturn err\n\t}")
}

func c(fname, tname, orderconvert string) {
	fmt.Printf("\t%s = %s(binary.LittleEndian.%s(bs))\n", fname, tname, orderconvert)
}

func unmarshalField(fname, tname string) {
	switch tname {
	case "int", "int64", "uint64":
		r(8)
		c(fname, tname, "Uint64")
	case "int32", "uint32":
		r(4)
		c(fname, tname, "Uint32")
	case "int16", "uint16":
		r(2)
		c(fname, tname, "Uint16")
	case "int8", "uint8":
		r(1)
		fmt.Printf("\t%s = b[0]\n", fname)
	default:
		fmt.Printf("\tUnmarshal%s(&%s, w)\n", tname, fname)
	}
}

func marshalField(fname, tname string) {
	switch tname {
	case "int", "int64", "uint64":
		b(8)
		bs(fname, "uint64", "Uint64")
		wbs()
	case "int32", "uint32":
		b(4)
		bs(fname, "uint32", "Uint32")
		wbs()
	case "int16", "uint16":
		b(2)
		bs(fname, "uint16", "Uint16")
		wbs()
	case "int8", "uint8":
		b(1)
		fmt.Printf("\tb[0] = byte(%s)\n", fname)
		wbs()
	default:
		fmt.Printf("\tMarshal%s(&%s, w)\n", tname, fname)
	}
}


func walkContents(st *ast.StructType, pred string, funcname string, fn func(string, string)) {
	for _, f := range st.Fields.List {
		fname := f.Names[0].Name
		newpred := pred+"."+fname
		walkOne(f, newpred, funcname, fn)
	}
}

func walkOne(f *ast.Field, pred string, funcname string, fn func(string, string)) {
	switch f.Type.(type) {
	case *ast.Ident:
		t := f.Type.(*ast.Ident)
		fn(pred, t.Name)
	case *ast.SelectorExpr:
		se := f.Type.(*ast.SelectorExpr)
		fmt.Printf("\t%s.%s%s(&%s, w)\n",
			se.X, funcname, se.Sel.Name, pred)
	case *ast.ArrayType:
		s := f.Type.(*ast.ArrayType)
		e, ok := s.Len.(*ast.BasicLit)
		if !ok {
			panic("Bad literal in array decl")
		}
		
		len, _ := strconv.Atoi(e.Value) // check the error, lazybones
		//fmt.Println("Array len: ", len)
		for i := 0; i < len; i++ {
			fsub := fmt.Sprintf("%s[%d]", pred, i)
			pseudofield := &ast.Field{nil, nil, s.Elt, nil, nil}
			//fmt.Println("PF: ", pseudofield)
			walkOne(pseudofield, fsub, funcname, fn)
			//fmt.Println("Type of elt: ", reflect.TypeOf(s.Elt))
			//unmarshalContents(s.Elt
			//unmarshalField(fsub, elt.Name, pred)
		}
	default:
		panic("Unknown type in struct")
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
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return
	}

	//fmt.Println("ts: ", typeName)
	fmt.Printf("func Marshal%s(t *%s, w io.Writer) {\n", typeName, typeName)
	fmt.Println("\tvar b [8]byte")
	fmt.Println("\tbs := b[:8]")
	resetb()
	//fmt.Printf("tstype: ", reflect.TypeOf(ts.Type))
	walkContents(st, "t", "Marshal", marshalField)
	fmt.Println("}\n")


	fmt.Printf("func Unmarshal%s(t *%s, r io.Reader) error {\n", typeName, typeName)
	fmt.Println("\tvar b [8]byte")
	fmt.Println("\tbs := b[:8]")
	resetb()
	walkContents(st, "t", "Unmarshal", unmarshalField)
	fmt.Println("\treturn nil\n}\n")
	return
}

func (bf *Binidl) PrintGo() {
	walk(bf.ast, structmap)
}

func let_me_keep_reflect_loaded_please() {
	fmt.Println(reflect.TypeOf(0))
}
