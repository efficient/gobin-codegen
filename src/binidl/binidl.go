package binidl

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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
	fmt.Println("package", ast.Name.Name)
    fmt.Println("")
    fmt.Println("import (\n\t\"io\"\n\t\"encoding/binary\"\n)\n")
	return &Binidl{ast, fset}
}

var cur_b_marshal_state int = 0

func resetb() {
	cur_b_marshal_state = 8
}

func b(n int, pad string) {
	if (n != cur_b_marshal_state) {
		fmt.Printf("%sbs = b[:%d]\n", pad, n)
	}
	cur_b_marshal_state = n
}
func bs(fname, tname, orderconvert string, pad string) {
	fmt.Printf("%sbinary.LittleEndian.Put%s(bs, %s(%s))\n", pad, orderconvert, tname, fname)
}
func wbs(pad string) {
	fmt.Printf("%sw.Write(bs)\n", pad)
}

func r(n int, pad string) {
	b(n, pad)
	fmt.Printf("%sif _, err := io.ReadFull(r, bs); err != nil {\n", pad)
	fmt.Printf("%s\treturn err\n%s}\n", pad, pad)
}

func c(fname, tname, orderconvert string, pad string) {
	fmt.Printf("%s%s = %s(binary.LittleEndian.%s(bs))\n", pad, fname, tname, orderconvert)
}

func unmarshalField(fname, tname, pad string) {
	tconv := tname
	if mapped, ok := typemap[tname]; ok {
		tconv = mapped
	}
	switch tconv {
	case "int", "int64", "uint64":
		r(8, pad)
		c(fname, tname, "Uint64", pad)
	case "int32", "uint32":
		r(4, pad)
		c(fname, tname, "Uint32", pad)
	case "int16", "uint16":
		r(2, pad)
		c(fname, tname, "Uint16", pad)
	case "int8", "uint8":
		r(1, pad)
		fmt.Printf("%s%s = b[0]\n", pad, fname)
	default:
		fmt.Printf("%s%s.Unmarshal(r)\n", pad, fname)
	}
}

func marshalField(fname, tname, pad string) {
	if mapped, ok := typemap[tname]; ok {
		tname = mapped
	}

	switch tname {
	case "int", "int64", "uint64":
		b(8, pad)
		bs(fname, "uint64", "Uint64", pad)
		wbs(pad)
	case "int32", "uint32":
		b(4, pad)
		bs(fname, "uint32", "Uint32", pad)
		wbs(pad)
	case "int16", "uint16":
		b(2, pad)
		bs(fname, "uint16", "Uint16", pad)
		wbs(pad)
	case "int8", "uint8":
		b(1, pad)
		fmt.Printf("%sb[0] = byte(%s)\n", pad, fname)
		wbs(pad)
	default:
		fmt.Printf("%s%s.Marshal(w)\n", pad, fname)
	}
}


func walkContents(st *ast.StructType, pred string, funcname string, fn func(string, string, string)) {
	for _, f := range st.Fields.List {
		fname := f.Names[0].Name
		newpred := pred+"."+fname
		walkOne(f, newpred, funcname, fn, "\t")
	}
}

var index_depth int = 0
func get_index_str() string {
	indexes := []string{"i", "j", "k", "ii", "jj", "kk"}
	if index_depth > 5 {
		panic("Array nesting depth too large.  Lazy programmer bites again.")
	}
	i := index_depth
	index_depth++
	return indexes[i]
}
func free_index_str() {
	index_depth--
}

func walkOne(f *ast.Field, pred string, funcname string, fn func(string, string, string), pad string) {
    ioid := "w"
    if funcname == "Unmarshal" {
        ioid = "r"
    }
	switch f.Type.(type) {
	case *ast.Ident:
		t := f.Type.(*ast.Ident)
		fn(pred, t.Name, pad)
	case *ast.SelectorExpr:
		//se := f.Type.(*ast.SelectorExpr)
		//fmt.Printf("%s%s.%s%s(&%s, w)\n",
		//	pad, se.X, funcname, se.Sel.Name, pred)
		fmt.Printf("%s%s.%s(%s)\n",
			pad, pred, funcname, ioid)
	case *ast.ArrayType:
		s := f.Type.(*ast.ArrayType)
		e, ok := s.Len.(*ast.BasicLit)
		if !ok {
			panic("Bad literal in array decl")
		}
		
		len, err := strconv.Atoi(e.Value)
		if err != nil {
			panic("Bad array length value.  Must be a simple int.")
		}
		i := get_index_str()
		fmt.Printf("%sfor %s := 0; %s < %d; %s++ {\n", pad, i, i, len, i)

		// Might want to unroll if len is only 2.
		//for i := 0; i < len; i++ {
		fsub := fmt.Sprintf("%s[%s]", pred, i)
		pseudofield := &ast.Field{nil, nil, s.Elt, nil, nil}
		walkOne(pseudofield, fsub, funcname, fn, pad+"\t")
		fmt.Printf("%s}\n", pad)
		free_index_str()
	default:
		panic("Unknown type in struct")
	}
}

var typemap map[string]string = make(map[string]string)
var native_type map[string]bool = map[string]bool { "int64":true,
	"uint64":true, "int":true, "int32":true, "uint32":true,
	"int16":true, "uint16":true, "int8":true, "uint8":true, }

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
		//fmt.Println("Type of type is ", reflect.TypeOf(ts.Type))
		if id, ok := ts.Type.(*ast.Ident); ok {
			tname := id.Name
			if native_type[tname] {
				typemap[typeName] = tname
				return
			}
		}
		panic("Can't handle decl!")
	}

	//fmt.Println("ts: ", typeName)
	fmt.Printf("func (t *%s) Marshal(w io.Writer) {\n", typeName)
	fmt.Println("\tvar b [8]byte")
	fmt.Println("\tbs := b[:8]")
	resetb()
	//fmt.Printf("tstype: ", reflect.TypeOf(ts.Type))
	walkContents(st, "t", "Marshal", marshalField)
	fmt.Println("}\n")


	fmt.Printf("func (t *%s) Unmarshal(r io.Reader) error {\n", typeName)
	fmt.Println("\tvar b [8]byte")
	fmt.Println("\tbs := b[:8]")
	resetb()
	walkContents(st, "t", "Unmarshal", unmarshalField)
	fmt.Println("\treturn nil\n}\n")
	return
}

func (bf *Binidl) PrintGo() {
	for _, d := range bf.ast.Decls {
		structmap(d)
	}
}

func let_me_keep_reflect_loaded_please() {
	fmt.Println(reflect.TypeOf(0))
}
