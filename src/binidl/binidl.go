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
	b(n)
	fmt.Println("\tif _, err := io.ReadFull(r, bs); err != nil {")
	fmt.Println("\t\treturn err\n\t}")
}

func c(f, tname, orderconvert string) {
	fmt.Printf("\t%s = %s(binary.LittleEndian.%s(bs))\n", f, tname, orderconvert)
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

// silly.  This completely duplicates marshalContents.  Fix me, Dave.
func walkContents(st *ast.StructType, pred string, fn func(*ast.Field, string)) {
	for _, f := range st.Fields.List {
		fname := f.Names[0].Name
		newpred := pred+"."+fname
		fn(f, newpred)
	}
}

func unmarshalOne(f *ast.Field, pred string) {
	switch f.Type.(type) {
	case *ast.Ident:
		t := f.Type.(*ast.Ident)
		unmarshalField(pred, t.Name)
	case *ast.SelectorExpr:
		se := f.Type.(*ast.SelectorExpr)
		fmt.Printf("\t%s.Unmarshal%s(&%s, w)\n",
			se.X, se.Sel.Name, pred)
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
			unmarshalOne(pseudofield, fsub)
			//fmt.Println("Type of elt: ", reflect.TypeOf(s.Elt))
			//unmarshalContents(s.Elt
			//unmarshalField(fsub, elt.Name, pred)
		}
	default:
		panic("Unknown type in struct")
	}
}

func marshalField(fname, tname string) {
	switch tname {
	case "int", "int64", "uint64":
		b(8)
		bs("binary.LittleEndian.PutUint64", "uint64", fname)
		wbs()
	case "int32", "uint32":
		b(4)
		bs("binary.LittleEndian.PutUint32", "uint32", fname)
		wbs()
	case "int16", "uint16":
		b(2)
		bs("binary.LittleEndian.PutUint16", "uint16", fname)
		wbs()
	case "int8", "uint8":
		b(1)
		fmt.Printf("\tb[0] = byte(%s)\n", fname)
		wbs()
	default:
		fmt.Printf("\tMarshal%s(&%s, w)\n", tname, fname)
	}
}


func marshalOne(f *ast.Field, pred string) {
	switch f.Type.(type) {
	case *ast.Ident:
		t := f.Type.(*ast.Ident)
		marshalField(pred, t.Name)
	case *ast.SelectorExpr:
		se := f.Type.(*ast.SelectorExpr)
		fmt.Printf("\t%s.Marshal%s(&%s, w)\n",
			se.X, se.Sel.Name, pred)
	case *ast.ArrayType:
		s := f.Type.(*ast.ArrayType)
		e, ok := s.Len.(*ast.BasicLit)
		if !ok {
			panic("Bad literal in array decl")
		}
		
		len, _ := strconv.Atoi(e.Value) // check the error, lazybones
		for i := 0; i < len; i++ {
			fsub := fmt.Sprintf("%s[%d]", pred, i)
			pseudofield := &ast.Field{nil, nil, s.Elt, nil, nil}
			marshalOne(pseudofield, fsub)
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
	//fmt.Printf("tstype: ", reflect.TypeOf(ts.Type))
	walkContents(st, "t", marshalOne)
	fmt.Println("}\n")

	fmt.Printf("func Unmarshal%s(t *%s, r io.Reader) error {\n", typeName, typeName)
	fmt.Println("\tvar b [8]byte")
	fmt.Println("\tvar bs []byte")
	walkContents(st, "t", unmarshalOne)
	fmt.Println("\treturn nil\n}\n")
	return
}

func (bf *Binidl) PrintGo() {
	walk(bf.ast, structmap)
}

func let_me_keep_reflect_loaded_please() {
	fmt.Println(reflect.TypeOf(0))
}
