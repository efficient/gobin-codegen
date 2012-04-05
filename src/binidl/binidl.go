package binidl

import (
	"fmt"
	"go/ast"
	"bytes"
	"go/parser"
	"go/token"
	"io"
	"os"
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

var cur_b_marshal_state int = 0

func resetb() {
	cur_b_marshal_state = 10
}

func setbs(b io.Writer, n int, pad string) {
	if (n != cur_b_marshal_state) {
		fmt.Fprintf(b, "%sbs = b[:%d]\n", pad, n)
	}
	cur_b_marshal_state = n
}
func bs(b io.Writer, fname, tname, orderconvert string, pad string) {
	fmt.Fprintf(b, "%sbinary.LittleEndian.Put%s(bs, %s(%s))\n", pad, orderconvert, tname, fname)
}
func wbs(b io.Writer, pad string) {
	fmt.Fprintf(b, "%sw.Write(bs)\n", pad)
}

func r(b io.Writer, n int, pad string) {
	setbs(b, n, pad)
	fmt.Fprintf(b, "%sif _, err := io.ReadFull(r, bs); err != nil {\n", pad)
	fmt.Fprintf(b, "%s\treturn err\n%s}\n", pad, pad)
}

func c(b io.Writer, fname, tname, orderconvert, pad string) {
	fmt.Fprintf(b, "%s%s = %s(binary.LittleEndian.%s(bs))\n", pad, fname, tname, orderconvert)
}

func unmarshalField(b io.Writer, fname, tname, pad string) {
	tconv := tname
	if mapped, ok := typemap[tname]; ok {
		tconv = mapped
	}
	switch tconv {
	case "int", "int64", "uint64":
		r(b, 8, pad)
		c(b, fname, tname, "Uint64", pad)
	case "int32", "uint32":
		r(b, 4, pad)
		c(b, fname, tname, "Uint32", pad)
	case "int16", "uint16":
		r(b, 2, pad)
		c(b, fname, tname, "Uint16", pad)
	case "int8", "uint8", "byte":
		r(b, 1, pad)
		fmt.Fprintf(b, "%s%s = %s(b[0])\n", pad, fname, tname)
	default:
		fmt.Fprintf(b, "%s%s.Unmarshal(r)\n", pad, fname)
	}
}

func marshalField(b io.Writer, fname, tname, pad string) {
	if mapped, ok := typemap[tname]; ok {
		tname = mapped
	}

	switch tname {
	case "int", "int64", "uint64":
		setbs(b, 8, pad)
		bs(b, fname, "uint64", "Uint64", pad)
		wbs(b, pad)
	case "int32", "uint32":
		setbs(b, 4, pad)
		bs(b, fname, "uint32", "Uint32", pad)
		wbs(b, pad)
	case "int16", "uint16":
		setbs(b, 2, pad)
		bs(b, fname, "uint16", "Uint16", pad)
		wbs(b, pad)
	case "int8", "uint8", "byte":
		setbs(b, 1, pad)
		fmt.Fprintf(b, "%sb[0] = byte(%s)\n", pad, fname)
		wbs(b, pad)
	default:
		fmt.Fprintf(b, "%s%s.Marshal(w)\n", pad, fname)
	}
}


func walkContents(b io.Writer, st *ast.StructType, pred string, funcname string, fn func(io.Writer, string, string, string)) {
	for _, f := range st.Fields.List {
		fname := f.Names[0].Name
		newpred := pred+"."+fname
		walkOne(b, f, newpred, funcname, fn, "\t")
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

var need_readbyte bool = false
var need_bufio = false

func walkOne(b io.Writer, f *ast.Field, pred string, funcname string, fn func(io.Writer, string, string, string), pad string) {
    ioid := "w"
    if funcname == "Unmarshal" {
        ioid = "r"
    }
	switch f.Type.(type) {
	case *ast.Ident:
		t := f.Type.(*ast.Ident)
		fn(b, pred, t.Name, pad)
	case *ast.SelectorExpr:
		//se := f.Type.(*ast.SelectorExpr)
		//fmt.Printf("%s%s.%s%s(&%s, w)\n",
		//	pad, se.X, funcname, se.Sel.Name, pred)
		fmt.Fprintf(b, "%s%s.%s(%s)\n",
			pad, pred, funcname, ioid)
	case *ast.ArrayType:
		s := f.Type.(*ast.ArrayType)
		i := get_index_str()
		fmt.Fprintf(b, "%s{\n", pad)
		if s.Len == nil {
			// If we are unmarshaling we need to allocate.
			if ioid == "r" {
				need_readbyte = true
				need_bufio = true
				fmt.Fprintf(b, "%slen, err := binary.ReadVarint(r)\n", pad)
				fmt.Fprintf(b, "%sif err != nil {\n", pad)
				fmt.Fprintf(b, "%s\treturn err\n", pad)
				fmt.Fprintf(b, "%s}\n", pad)
				fmt.Fprintf(b, "%s%s = make([]%s, len)\n", pad, pred, s.Elt)
			} else {
				setbs(b, 10, pad)
				fmt.Fprintf(b, "%slen := int64(len(%s))\n", pad, pred)
				fmt.Fprintf(b, "%sif wlen := binary.PutVarint(bs, len); wlen >= 0 {\n", pad)
				fmt.Fprintf(b, "%s\tw.Write(b[0:wlen])\n", pad)
				fmt.Fprintf(b, "%s}\n", pad)
			}
			fmt.Fprintf(b, "%sfor %s := int64(0); %s < len; %s++ {\n", pad, i, i, i)
		} else {
			e, ok := s.Len.(*ast.BasicLit)
			if !ok {
				panic("Bad literal in array decl")
			}
			len, err := strconv.Atoi(e.Value)
			if err != nil {
				panic("Bad array length value.  Must be a simple int.")
			}
			fmt.Fprintf(b, "%sfor %s := 0; %s < %d; %s++ {\n", pad, i, i, len, i)
		}
		
		fsub := fmt.Sprintf("%s[%s]", pred, i)
		pseudofield := &ast.Field{nil, nil, s.Elt, nil, nil}
		walkOne(b, pseudofield, fsub, funcname, fn, pad+"\t")
		fmt.Fprintf(b, "%s}\n", pad)
		fmt.Fprintf(b, "%s}\n", pad)
		free_index_str()
	default:
		panic("Unknown type in struct")
	}
}

var typemap map[string]string = make(map[string]string)
var native_type map[string]bool = map[string]bool { "int64":true,
	"uint64":true, "int":true, "int32":true, "uint32":true,
	"int16":true, "uint16":true, "int8":true, "uint8":true, }

func structmap(out io.Writer, n interface{}) {
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

	b := new(bytes.Buffer)

	need_readbyte = false
	//fmt.Println("ts: ", typeName)
	fmt.Fprintf(out, "func (t *%s) Marshal(w io.Writer) {\n", typeName)
	fmt.Fprintln(out, "\tvar b [10]byte")
	fmt.Fprintln(out, "\tbs := b[:10]")
	resetb()
	//fmt.Printf("tstype: ", reflect.TypeOf(ts.Type))
	walkContents(b, st, "t", "Marshal", marshalField)
	b.WriteTo(out)
	fmt.Fprintln(out, "}\n")

	// This is a good thing to optimize in the future:  Adding a bufio slows
	// unmarshaling down by a fair bit and is only needed for certain types
	// of structs.
	b.Reset()
	resetb()
	walkContents(b, st, "t", "Unmarshal", unmarshalField)
	paramname := "r"
	if need_readbyte {
		paramname = "rr"
	}
	fmt.Fprintf(out, "func (t *%s) Unmarshal(%s io.Reader) error {\n", typeName, paramname)
	if need_readbyte {
		fmt.Fprintln(out, "\tr := bufio.NewReader(rr)\n")
	}
	fmt.Fprintln(out, "\tvar b [10]byte")
	fmt.Fprintln(out, "\tbs := b[:10]")
	b.WriteTo(out)
	fmt.Fprintln(out, "\treturn nil\n}\n")
	return
}

func (bf *Binidl) PrintGo() {
	b := new(bytes.Buffer)
	for _, d := range bf.ast.Decls {
		structmap(b, d)
	}
	fmt.Println("package", bf.ast.Name.Name)
	fmt.Println("")
	fmt.Println("import (")
	if need_bufio {
		fmt.Println("\t\"bufio\"")
	}
	fmt.Println("\t\"io\"\n\t\"encoding/binary\"")
	fmt.Println(")")
	b.WriteTo(os.Stdout)
}

func let_me_keep_reflect_loaded_please() {
	fmt.Println(reflect.TypeOf(0))
}
