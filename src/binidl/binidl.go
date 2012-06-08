package binidl

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

type Binidl struct {
	ast       *ast.File
	fset      *token.FileSet
	bigEndian bool
}

const (
	STATICMAX = 64 // Max size of an object for which we marshal into a stack-allocated local buffer
)

func NewBinidl(filename string, bigEndian bool) *Binidl {
	fset := token.NewFileSet()
	ast, err := parser.ParseFile(fset, filename, nil, 0) // scanner.InsertSemis)
	if err != nil {
		fmt.Println("Error parsing", filename, ":", err)
		return nil
	}
	return &Binidl{ast, fset, bigEndian}
}

func setbs(b io.Writer, n int, es *EmitState, force bool) {
	if es.isStatic && !force {
		return
	}
	if n != es.curBSize {
		fmt.Fprintf(b, "bs = b[:%d]\n", n)
	}
	es.curBSize = n
}

func unmarshalField(b io.Writer, fname, tname string, es *EmitState) {
	tconv := tname
	if mapped, ok := typemap[tname]; ok {
		tconv = mapped
	}

	ti, ok := typedb[tconv]
	if !ok {
		fmt.Fprintf(b, "%s.Unmarshal(wire)\n", fname)
		return
	}

	source := "b"
	if !es.isStatic {
		source = "bs"
		setbs(b, ti.Size, es, false)
		fmt.Fprintf(b, "if _, err := io.ReadAtLeast(wire, bs, %d); err != nil {\n", ti.Size)
		fmt.Fprintf(b, " return err\n")
		fmt.Fprintf(b, "}\n")
	}

	bstart := es.Bstart(ti.Size)

	if ildf, found := inlineDecode[ti.EncodesAs]; found {
		ild := ildf(source, bstart, es)
		fmt.Fprintf(b, "%s = %s(%s)\n", fname, tname, ild)
	} else {
		endian := "Little"
		if es.bigEndian {
			endian = "Big"
		}
		df := fmt.Sprintf(decodeFunc[ti.EncodesAs], endian)
		if !es.isStatic {
			fmt.Fprintf(b, "%s = %s(%s(bs))\n", fname, tname, df)
		} else {
			fmt.Fprintf(b, "%s = %s(%s(b[%d:%d]))\n", fname, tname, df, bstart, bstart+ti.Size)
		}
	}
}

func marshalField(b io.Writer, fname, tname string, es *EmitState) {
	if mapped, ok := typemap[tname]; ok {
		tname = mapped
	}
	ti, ok := typedb[tname]
	if !ok {
		fmt.Fprintf(b, "%s.Marshal(wire)\n", fname)
		return
	}

	setbs(b, ti.Size, es, false)
	ilef, found := inlineEncode[ti.EncodesAs]
	bstart := es.Bstart(ti.Size)
	encodefrom := "bs"
	if es.isStatic {
		encodefrom = "b"
	}
	if found {
		fmt.Fprintf(b, "%s\n", ilef(encodefrom, bstart, fname, es))
	} else {
		endian := "Little"
		if es.bigEndian {
			endian = "Big"
		}
		ef := fmt.Sprintf(encodeFunc[ti.EncodesAs], endian)
		if !es.isStatic {
			fmt.Fprintf(b, "%s(bs, %s(%s))\n", ef, ti.EncodesAs, fname)
		} else {
			bend := bstart + ti.Size
			fmt.Fprintf(b, "%s(b[%d:%d], %s(%s))\n", ef, bstart, bend, ti.EncodesAs, fname)
		}
	}
	if !es.isStatic {
		fmt.Fprintln(b, "wire.Write(bs)")
	}
}

func walkContents(b io.Writer, st *ast.StructType, pred string, funcname string, fn func(io.Writer, string, string, *EmitState), es *EmitState) {
	for _, f := range st.Fields.List {
		for _, fNameEnt := range f.Names {
			newpred := pred + "." + fNameEnt.Name
			walkOne(b, f, newpred, funcname, fn, es)
		}
	}
}

const (
	MARSHAL = iota
	UNMARSHAL
)

type EmitState struct {
	op           int // MARSHAL, UNMARSHAL
	nextIdx      int
	isStatic     bool
	staticOffset int
	alenIdx      int
	curBSize     int
	bigEndian    bool // TODO:  This is duplicated now... integrate better.
	tmp32exists  bool
}

func (es *EmitState) getNewAlen() string {
	es.alenIdx++
	return fmt.Sprintf("alen%d", es.alenIdx)
}

func (es *EmitState) getIndexStr() string {
	indexes := []string{"i", "j", "k"}
	repeats := (es.nextIdx / len(indexes)) + 1
	pos := es.nextIdx % len(indexes)
	es.nextIdx++
	return strings.Repeat(indexes[pos], repeats)
}

func (es *EmitState) freeIndexStr() {
	es.nextIdx--
}

func (es *EmitState) Bstart(n int) int {
	if !es.isStatic {
		return 0
	}
	o := es.staticOffset
	es.staticOffset += n
	return o
}

var need_bufio = false

var typemap map[string]string = make(map[string]string)

type TypeInfo struct {
	Name      string
	Size      int
	EncodesAs string
}

var encodeFunc map[string]string = map[string]string{
	"uint64": "binary.%sEndian.PutUint64",
	"uint32": "binary.%sEndian.PutUint32",
	"uint16": "binary.%sEndian.PutUint16",
}

type encodefunc func(string, int, string, *EmitState) string

func ilByteOut(b string, offset int, target string, es *EmitState) string {
	return fmt.Sprintf("%s[%d] = byte(%s)", b, offset, target)
}

func ilUint16Out(b string, offset int, target string, es *EmitState) string {
	if !es.bigEndian {
		return fmt.Sprintf("%s[%d] = byte(%s)\n%s[%d] = byte(%s >> 8)",
			b, offset, target, b, offset+1, target)
	}
	return fmt.Sprintf("%s[%d] = byte(%s >> 8)\n%s[%d] = byte(%s)",
		b, offset, target, b, offset+1, target)
}

func ilUint32Out(b string, offset int, target string, es *EmitState) string {
	tmp32 := ""
	if !es.tmp32exists {
		tmp32 = fmt.Sprintf("tmp32 := %s\n", target)
		es.tmp32exists = true
	} else {
		tmp32 = fmt.Sprintf("tmp32 = %s\n", target)
	}
	target = "tmp32"
	if !es.bigEndian {
		return fmt.Sprintf("%s%s[%d] = byte(%s)\n%s[%d] = byte(%s >> 8)\n%s[%d] = byte(%s >> 16)\n%s[%d] = byte(%s >> 24)",
			tmp32, b, offset, target, b, offset+1, target, b, offset+2, target, b, offset+3, target)
	}
	return fmt.Sprintf("%s%s[%d] = byte(%s >> 24)\n%s[%d] = byte(%s >> 16)\n%s[%d] = byte(%s >> 8)\n%s[%d] = byte(%s)",
		tmp32, b, offset, target, b, offset+1, target, b, offset+2, target, b, offset+3, target)
}

var inlineEncode map[string]encodefunc = map[string]encodefunc{
	"byte":   ilByteOut,
	"uint16": ilUint16Out,
	"uint32": ilUint32Out,
}

type decodefunc func(string, int, *EmitState) string

func ilByte(b string, offset int, es *EmitState) string {
	return fmt.Sprintf("%s[%d]", b, offset)
}

func ilUint16(b string, offset int, es *EmitState) string {
	if es.bigEndian {
		return fmt.Sprintf("((uint16(%s[%d]) << 8) | uint16(%s[%d]))", b, offset, b, offset+1)
	}
	return fmt.Sprintf("(uint16(%s[%d]) | uint16(%s[%d]) << 8)", b, offset, b, offset+1)
}

func ilUint32(b string, offset int, es *EmitState) string {
	if es.bigEndian {
		return fmt.Sprintf("((uint32(%s[%d]) << 24) | (uint32(%s[%d]) << 16)  | (uint32(%s[%d]) << 8) | uint32(%s[%d]))", b, offset, b, offset+1, b, offset+2, b, offset+3)
	}
	return fmt.Sprintf("(uint32(%s[%d]) | (uint32(%s[%d]) << 8)  | (uint32(%s[%d]) << 16) | (uint32(%s[%d]) << 24))", b, offset, b, offset+1, b, offset+2, b, offset+3)
}

var inlineDecode map[string]decodefunc = map[string]decodefunc{
	"byte":   ilByte,
	"uint16": ilUint16,
	"uint32": ilUint32,
}

var decodeFunc map[string]string = map[string]string{
	"uint64": "binary.%sEndian.Uint64",
	"uint32": "binary.%sEndian.Uint32",
	"uint16": "binary.%sEndian.Uint16",
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

func walkOne(b io.Writer, f *ast.Field, pred string, funcname string, fn func(io.Writer, string, string, *EmitState), es *EmitState) {
	switch f.Type.(type) {
	case *ast.Ident:
		t := f.Type.(*ast.Ident)
		_, is_mapped := typemap[t.Name]

		if dispatchTo, ok := globalDeclMap[t.Name]; !is_mapped && ok && es.isStatic {
			if strucType, ok := dispatchTo.Type.(*ast.StructType); ok {
				walkContents(b, strucType, pred, funcname, fn, es)
			} else {
				panic("Eek, a type I don't handle properly")
			}
		} else {
			fn(b, pred, t.Name, es)
		}
	case *ast.SelectorExpr:
		fmt.Fprintf(b, "%s.%s(wire)\n", pred, funcname)
	case *ast.ArrayType:
		s := f.Type.(*ast.ArrayType)
		i := es.getIndexStr()
		arrayLen := 0
		alenid := es.getNewAlen()
		if s.Len == nil {
			// If we are unmarshaling we need to allocate.
			if es.op == UNMARSHAL {
				fmt.Fprintf(b, "%s, err := binary.ReadVarint(wire)\n", alenid)
				fmt.Fprintf(b, "if err != nil {\n")
				fmt.Fprintf(b, "return err\n")
				fmt.Fprintf(b, "}\n")
				fmt.Fprintf(b, "%s = make([]%s, %s)\n", pred, s.Elt, alenid)
			} else {
				setbs(b, 10, es, false)
				fmt.Fprintf(b, "%s := int64(len(%s))\n", alenid, pred)
				fmt.Fprintf(b, "if wlen := binary.PutVarint(bs, %s); wlen >= 0 {\n", alenid)
				fmt.Fprintf(b, "wire.Write(b[0:wlen])\n")
				fmt.Fprintf(b, "}\n")
			}
			fmt.Fprintf(b, "for %s := int64(0); %s < %s; %s++ {\n", i, i, alenid, i)
		} else {
			e, ok := s.Len.(*ast.BasicLit)
			if !ok {
				panic("Bad literal in array decl")
			}
			var err error
			arrayLen, err = strconv.Atoi(e.Value)
			if err != nil {
				panic("Bad array length value.  Must be a simple int.")
			}
			if !es.isStatic {
				fmt.Fprintf(b, "for %s := 0; %s < %d; %s++ {\n", i, i, arrayLen, i)
			}
		}

		fsub := fmt.Sprintf("%s[%s]", pred, i)
		pseudofield := &ast.Field{Type: s.Elt}
		if es.isStatic {
			for idx := 0; idx < arrayLen; idx++ {
				fsub := fmt.Sprintf("%s[%d]", pred, idx)
				walkOne(b, pseudofield, fsub, funcname, fn, es)
			}
		} else {
			walkOne(b, pseudofield, fsub, funcname, fn, es)
			fmt.Fprintln(b, "}")
		}
		es.freeIndexStr()
	default:
		fmt.Println("Unknown type: ", f)
		panic("Unknown type in struct")
	}
}

type StructInfo struct {
	size         int
	maxSize      int
	firstSize    int
	varLen       bool
	mustDispatch bool
	totalSize    int // Including embedded types, if known
}

var structInfoMap map[string]*StructInfo

func mergeInfo(parent, child *StructInfo, childcount int) {
	if parent.firstSize == 0 {
		parent.firstSize = child.firstSize
	}
	if child.maxSize > parent.maxSize {
		parent.maxSize = child.maxSize
	}
	parent.varLen = parent.varLen || child.varLen
	parent.mustDispatch = parent.mustDispatch || child.mustDispatch

	if childcount > 0 {
		parent.size += child.size * childcount
		parent.totalSize += child.totalSize * childcount
	}

}

// Wouldn't it be nice to cache a lot of this? :-)
func analyze(n interface{}) (info *StructInfo) {
	info = new(StructInfo)
	switch n.(type) {
	case *ast.StructType:
		st := n.(*ast.StructType)
		for _, field := range st.Fields.List {
			for _ = range field.Names {
				mergeInfo(info, analyze(field), 1)
			}
		}
	case *ast.Field:
		f := n.(*ast.Field)
		switch f.Type.(type) {
		case *ast.Ident:
			tname := f.Type.(*ast.Ident).Name
			if mapped, ok := typemap[tname]; ok {
				tname = mapped
			}
			if tinfo, ok := typedb[tname]; ok {
				info.firstSize = tinfo.Size
				info.maxSize = tinfo.Size
				info.size = tinfo.Size
			} else {
				seinfo := analyzeType(tname)
				if seinfo != nil && seinfo.mustDispatch == false && seinfo.varLen == false {
					mergeInfo(info, seinfo, 1)
				} else {
					info.mustDispatch = true
				}
			}
		case *ast.SelectorExpr:
			info.mustDispatch = true
		case *ast.ArrayType:
			s := f.Type.(*ast.ArrayType)
			arraylen := 0
			if s.Len == nil {
				// If we are unmarshaling we need to allocate.
				info.varLen = true
				need_bufio = true // eventually just in info
			} else {
				e, ok := s.Len.(*ast.BasicLit)
				if !ok {
					panic("Bad literal in array decl")
				}
				var err error
				arraylen, err = strconv.Atoi(e.Value)
				if err != nil {
					panic("Bad array length value.  Must be a simple int.")
				}
			}

			pseudofield := &ast.Field{Type: s.Elt}
			mergeInfo(info, analyze(pseudofield), arraylen)
		default:
			fmt.Println("Unknown type in struct: ", f)
			panic("Unknown type in struct")
		}
	default:
		panic("Unknown ast type")
	}
	return
}

func analyzeType(typeName string) (info *StructInfo) {
	ts, ok := globalDeclMap[typeName]
	if !ok {
		return nil
	}

	if st, ok := ts.Type.(*ast.StructType); ok {
		info = analyze(st)
		return info
	}

	if id, ok := ts.Type.(*ast.Ident); ok {
		tname := id.Name
		if _, ok := typedb[tname]; ok {
			typemap[typeName] = tname
			return
		}
	}
	panic("Can't handle decl: " + typeName)
	return
}

func (bi *Binidl) structmap(out io.Writer, ts *ast.TypeSpec) {
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
	info := analyze(st)
	//fmt.Println("Analysis result: ", info)

	fmt.Fprintf(out, "func (t *%s) BinarySize() (nbytes int, sizeKnown bool) {\n", typeName)
	if !info.varLen && !info.mustDispatch {
		fmt.Fprintf(out, "  return %d, true\n", info.size)
	} else {
		fmt.Fprintf(out, "return 0, false\n")
	}
	fmt.Fprintln(out, "}")

	fmt.Fprintf(out, "type %sCache struct {\n", typeName)
	fmt.Fprintf(out, "  mu sync.Mutex\n")
	fmt.Fprintf(out, "  cache []*%s\n", typeName)
	fmt.Fprintf(out, "}\n\n")
	fmt.Fprintf(out, "func New%sCache() *%sCache {\nreturn &%sCache{}\n}\n\n", typeName, typeName, typeName)

	fmt.Fprintf(out, "func (p *%sCache) Get() *%s {\n", typeName, typeName)
	fmt.Fprintf(out, "var t *%s\n", typeName)
	fmt.Fprintf(out, "p.mu.Lock()\n")
	fmt.Fprintf(out, "if (len(p.cache) > 0) {\n")
	fmt.Fprintf(out, "  t = p.cache[len(p.cache)-1]\n")
	fmt.Fprintf(out, "  p.cache = p.cache[0:(len(p.cache)-1)]\n")
	fmt.Fprintf(out, "}\n")
	fmt.Fprintf(out, "p.mu.Unlock()\n")
	fmt.Fprintf(out, "if t == nil { t = &%s{} }\n", typeName)
	fmt.Fprintf(out, "return t")
	fmt.Fprintf(out, "}\n")

	// Currently relying on re-allocating any variable length arrays to handle
	// properly zeroing out reused data structures.
	fmt.Fprintf(out, "func (p *%sCache) Put(t *%s) {\n", typeName, typeName)
	fmt.Fprintf(out, "p.mu.Lock()\n")
	fmt.Fprintf(out, "p.cache = append(p.cache, t)\n")
	fmt.Fprintf(out, "p.mu.Unlock()\n")
	fmt.Fprintf(out, "}\n")

	mes := &EmitState{bigEndian: bi.bigEndian, op: MARSHAL}
	if info.size < STATICMAX && !info.varLen && !info.mustDispatch {
		mes.isStatic = true
	}
	blen := 8
	if info.varLen {
		blen = 10
	}
	if mes.isStatic {
		blen = info.size
	}
	fmt.Fprintf(out, "func (t *%s) Marshal(wire io.Writer) {\n", typeName)
	fmt.Fprintf(out, "var b [%d]byte\n", blen)
	if !mes.isStatic {
		fmt.Fprintf(out, "bs := b[:%d]\n", info.firstSize)
	}
	mes.curBSize = info.firstSize
	walkContents(out, st, "t", "Marshal", marshalField, mes)

	if mes.isStatic {
		fmt.Fprintln(out, "wire.Write(b[:])")
	}
	fmt.Fprintf(out, "}\n\n")

	ues := &EmitState{bigEndian: bi.bigEndian, op: UNMARSHAL, curBSize: info.firstSize}
	blen = 8
	if info.varLen {
		blen = 10
	}
	if info.size < 64 && !info.varLen && !info.mustDispatch {
		ues.isStatic = true
		blen = info.size
	}
	paramname := "wire"
	if info.varLen {
		paramname = "rr"
	}
	fmt.Fprintf(out, "func (t *%s) Unmarshal(%s io.Reader) error {\n", typeName, paramname)

	if info.varLen {
		fmt.Fprintln(out,
			`var wire byteReader
var ok bool
if wire, ok = rr.(byteReader); !ok {
    wire = bufio.NewReader(rr)
}`)
	}
	fmt.Fprintf(out, "var b [%d]byte\n", blen)
	if ues.isStatic {
		fmt.Fprintf(out, "bs := b[:]\n")
		fmt.Fprintf(out, "if _, err := io.ReadAtLeast(wire, bs, %d); err != nil {\n", blen)
		fmt.Fprintf(out, "return err\n}\n")
	} else {
		fmt.Fprintf(out, "bs := b[:%d]\n", info.firstSize)
	}
	walkContents(out, st, "t", "Unmarshal", unmarshalField, ues)
	fmt.Fprintf(out, "return nil\n}\n\n")
}

var globalDeclMap map[string]*ast.TypeSpec = make(map[string]*ast.TypeSpec)

func createGlobalDeclMap(decls []ast.Decl) {
	for _, d := range decls {
		if decl, ok := d.(*ast.GenDecl); ok && decl.Tok == token.TYPE {
			ts := decl.Specs[0].(*ast.TypeSpec)
			globalDeclMap[ts.Name.Name] = ts
		}
	}
}

func (bf *Binidl) PrintGo() {
	createGlobalDeclMap(bf.ast.Decls) // still a temporary hack
	rest := new(bytes.Buffer)
	for _, d := range globalDeclMap {
		bf.structmap(rest, d)
	}

	tf, err := ioutil.TempFile("", "gobin-codegen")
	if err != nil {
		panic(err)
	}
	tfname := tf.Name()
	defer os.Remove(tfname)
	defer tf.Close()

	fmt.Fprintln(tf, "package", bf.ast.Name.Name)
	imports := []string{"io", "encoding/binary", "sync"}
	if need_bufio {
		imports = append(imports, "bufio")
	}
	fmt.Fprintln(tf, "import (")
	for _, imp := range imports {
		fmt.Fprintf(tf, "\"%s\"\n", imp)
	}
	fmt.Fprintln(tf, ")")
	if need_bufio {
		fmt.Fprintln(tf, `type byteReader interface {
io.Reader
ReadByte() (c byte, err error)
}`)
	}
	// Output and then gofmt it to make it pretty and shiny.  And readable.
	rest.WriteTo(tf)
	tf.Sync()

	fset := token.NewFileSet()
	ast, err := parser.ParseFile(fset, tfname, nil, 0)
	if err != nil {
		panic(err.Error())
	}
	printer.Fprint(os.Stdout, fset, ast)
}
