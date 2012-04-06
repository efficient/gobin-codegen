package binidl

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/printer"
	"io"
	"io/ioutil"
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
var first_blen int = 0

func setbs(b io.Writer, n int) {
	if first_blen == 0 {
		first_blen = n
	} else {
		if n != cur_b_marshal_state {
			fmt.Fprintf(b, "bs = b[:%d]\n", n)
		}
	}
	cur_b_marshal_state = n
}
func bs(b io.Writer, fname, tname, orderconvert string) {
	fmt.Fprintf(b, "%s(bs, %s(%s))\n", orderconvert, tname, fname)
}

func r(b io.Writer, n int) {
	setbs(b, n)
	fmt.Fprintf(b, "if _, err := io.ReadFull(r, bs); err != nil {\n")
	fmt.Fprintf(b, "return err\n}\n")
}

func unmarshalField(b io.Writer, fname, tname string) {
	tconv := tname
	if mapped, ok := typemap[tname]; ok {
		tconv = mapped
	}

	ti, ok := typedb[tconv]
	if !ok {
		fmt.Fprintf(b, "%s.Unmarshal(r)\n", fname)
		return
	}

	r(b, ti.Size)
	if ti.Size == 1 {
		fmt.Fprintf(b, "%s = %s(b[0])\n", fname, tname)
	} else {
		fmt.Fprintf(b, "%s = %s(%s(bs))\n", fname, tname, decodeFunc[ti.EncodesAs])
	}
}

func marshalField(b io.Writer, fname, tname string) {
	if mapped, ok := typemap[tname]; ok {
		tname = mapped
	}

	ti, ok := typedb[tname]
	if !ok {
		fmt.Fprintf(b, "%s.Marshal(w)\n", fname)
		return
	}

	setbs(b, ti.Size)
	if ti.Size == 1 {
		fmt.Fprintf(b, "b[0] = byte(%s)\n", fname)
	} else {
		bs(b, fname, ti.EncodesAs, encodeFunc[ti.EncodesAs])
	}
	fmt.Fprintf(b, "w.Write(bs)\n")
}

func walkContents(b io.Writer, st *ast.StructType, pred string, funcname string, fn func(io.Writer, string, string)) {
	for _, f := range st.Fields.List {
		fname := f.Names[0].Name
		newpred := pred + "." + fname
		walkOne(b, f, newpred, funcname, fn)
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

var typemap map[string]string = make(map[string]string)

type TypeInfo struct {
	Name      string
	Size      int
	EncodesAs string
}

var encodeFunc map[string]string = map[string]string{
	"uint64": "binary.LittleEndian.PutUint64",
	"uint32": "binary.LittleEndian.PutUint32",
	"uint16": "binary.LittleEndian.PutUint16",
}

var decodeFunc map[string]string = map[string]string{
	"uint64": "binary.LittleEndian.Uint64",
	"uint32": "binary.LittleEndian.Uint32",
	"uint16": "binary.LittleEndian.Uint16",
}

var typedb map[string]TypeInfo = map[string]TypeInfo{
	"int":    {"int", 8, "uint64"},
	"uint64": {"uint64", 8, "uint64"},
	"int64":  {"int64", 8, "uint64"},
	"int32":  {"int32", 4, "uint32"},
	"uint32": {"uint32", 4, "uint32"},
	"int16":  {"int16", 2, "uint16"},
	"uint16": {"uint16", 2, "uint16"},
	"int8":   {"int8", 1, "byte"},
	"uint8":  {"uint8", 1, "byte"},
	"byte":   {"byte", 1, "byte"},
}

func walkOne(b io.Writer, f *ast.Field, pred string, funcname string, fn func(io.Writer, string, string)) {
	ioid := "w"
	if funcname == "Unmarshal" {
		ioid = "r"
	}
	switch f.Type.(type) {
	case *ast.Ident:
		t := f.Type.(*ast.Ident)
		fn(b, pred, t.Name)
	case *ast.SelectorExpr:
		//se := f.Type.(*ast.SelectorExpr)
		//fmt.Printf("%s.%s%s(&%s, w)\n",
		//	se.X, funcname, se.Sel.Name, pred)
		fmt.Fprintf(b, "%s.%s(%s)\n",
			pred, funcname, ioid)
	case *ast.ArrayType:
		s := f.Type.(*ast.ArrayType)
		i := get_index_str()
		fmt.Fprintf(b, "{\n")
		if s.Len == nil {
			// If we are unmarshaling we need to allocate.
			need_readbyte = true
			need_bufio = true
			if ioid == "r" {
				fmt.Fprintf(b, "len, err := binary.ReadVarint(r)\n")
				fmt.Fprintf(b, "if err != nil {\n")
				fmt.Fprintf(b, "return err\n")
				fmt.Fprintf(b, "}\n")
				fmt.Fprintf(b, "%s = make([]%s, len)\n", pred, s.Elt)
			} else {
				setbs(b, 10)
				fmt.Fprintf(b, "len := int64(len(%s))\n", pred)
				fmt.Fprintf(b, "if wlen := binary.PutVarint(bs, len); wlen >= 0 {\n")
				fmt.Fprintf(b, "w.Write(b[0:wlen])\n")
				fmt.Fprintf(b, "}\n")
			}
			fmt.Fprintf(b, "for %s := int64(0); %s < len; %s++ {\n", i, i, i)
		} else {
			e, ok := s.Len.(*ast.BasicLit)
			if !ok {
				panic("Bad literal in array decl")
			}
			len, err := strconv.Atoi(e.Value)
			if err != nil {
				panic("Bad array length value.  Must be a simple int.")
			}
			fmt.Fprintf(b, "for %s := 0; %s < %d; %s++ {\n", i, i, len, i)
		}

		fsub := fmt.Sprintf("%s[%s]", pred, i)
		pseudofield := &ast.Field{nil, nil, s.Elt, nil, nil}
		walkOne(b, pseudofield, fsub, funcname, fn)
		fmt.Fprintf(b, "}\n}\n")
		free_index_str()
	default:
		panic("Unknown type in struct")
	}
}

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
			if _, ok := typedb[tname]; ok {
				typemap[typeName] = tname
				return
			}
		}
		panic("Can't handle decl!")
	}

	b := new(bytes.Buffer)

	need_readbyte = false
	first_blen = 0
	//fmt.Println("ts: ", typeName)
	walkContents(b, st, "t", "Marshal", marshalField)
	blen := 8
	if need_readbyte {
		blen = 10
	}
	fmt.Fprintf(out, "func (t *%s) Marshal(w io.Writer) {\n", typeName)
	fmt.Fprintf(out, "var b [%d]byte\n", blen)
	fmt.Fprintf(out, "bs := b[:%d]\n", first_blen)

	b.WriteTo(out)
	fmt.Fprintln(out, "}\n")

	b.Reset()
	first_blen = 0
	walkContents(b, st, "t", "Unmarshal", unmarshalField)
	paramname := "r"
	if need_readbyte {
		paramname = "rr"
	}
	fmt.Fprintf(out, "func (t *%s) Unmarshal(%s io.Reader) error {\n", typeName, paramname)
	
	if need_readbyte {
		fmt.Fprintln(out, `
var r byteReader
if rrr, ok := rr.(byteReader); ok {
    r = rrr
} else {
    r = bufio.NewReader(rr)
}`)
	}
	fmt.Fprintf(out, "var b [%d]byte\n", blen)
	fmt.Fprintf(out, "bs := b[:%d]\n", first_blen)
	cur_b_marshal_state = first_blen
	b.WriteTo(out)
	fmt.Fprintln(out, "return nil\n}\n")
	return
}

func (bf *Binidl) PrintGo() {
	rest := new(bytes.Buffer)
	for _, d := range bf.ast.Decls {
		structmap(rest, d)
	}
	b := new(bytes.Buffer)
	fmt.Fprintln(b, "package", bf.ast.Name.Name)
	imports := []string{"io", "encoding/binary"}
	if need_bufio {
		imports = append(imports, "bufio")
	}
	fmt.Fprintln(b, "import (")
	for _, imp := range imports {
		fmt.Fprintf(b, "\"%s\"\n", imp)
	}
	fmt.Fprintln(b, ")")
	if need_bufio {
		fmt.Fprintln(b, `type byteReader interface {
io.Reader
ReadByte() (c byte, err error)
}`)
	}
	// Output and then gofmt it to make it pretty and shiny.  And readable.
	tf, _ := ioutil.TempFile("", "gobin-codegen")
	tfname := tf.Name()
	defer os.Remove(tfname)
	defer tf.Close()

	b.WriteTo(tf)
	rest.WriteTo(tf)
	tf.Sync()
	
	fset := token.NewFileSet()
	ast, err := parser.ParseFile(fset, tfname, nil, 0)
	if err != nil {
		panic(err.Error())
	}
	printer.Fprint(os.Stdout, fset, ast)
}

func let_me_keep_reflect_loaded_please() {
	fmt.Println(reflect.TypeOf(0))
}
