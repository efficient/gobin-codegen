package encodedemo

import (
	"testing"
	"bytes"
	"fmt"
	"encoding/binary"
	"encoding/gob"
)

var d *Demostruct = &Demostruct{1, 2, [4]int16{9, 9, 9, 9}}
var e *Demostruct = &Demostruct{0, 0, [4]int16{0, 0, 0, 0}}
var buf *bytes.Buffer = new(bytes.Buffer)

func TestE(t *testing.T) {
	buf.Reset()
	binary.Write(buf, binary.LittleEndian, d)
	fmt.Printf("bin: % x\n", buf.Bytes())
	buf.Reset()
	d.Marshal(buf)
	fmt.Printf("gen: % x\n", buf.Bytes())
	fmt.Println("")
}

func TestD(t *testing.T) {
	buf.Reset()
	binary.Write(buf, binary.LittleEndian, d)
	e.Unmarshal(buf)
	fmt.Println("Demostruct: ", e)
}

var s *Sliced = &Sliced{1, []int8{2, 3, 4}}
func TestSliced(t *testing.T) {
	buf.Reset()
	s.Marshal(buf)
	fmt.Print("Marshaled: ", s, " into ")
	fmt.Printf("% x\n", buf.Bytes())
}
func TestSliceUnmarshal(t *testing.T) {
	buf.Reset()
	s.Marshal(buf)
	s2 := &Sliced{0, nil}
	s2.Unmarshal(buf)
	fmt.Println("Unmarshaled: ", s2)
}

func TestEmbedded(t *testing.T) {
	x := &HasEmbedded{1, 2, IsEmbedded{3, 4}}
	buf.Reset()
	x.Marshal(buf)
	y := &HasEmbedded{}
	y.Unmarshal(buf)
	if (y.A != 1 || y.B != 2 || y.C.X != 3 || y.C.Y != 4) {
		t.Fatalf("Embedded struct test failed")
	}
}


func TestEmbedded2(t *testing.T) {
		x := HasEmbedded2{1, 2, IsEmbedded2{3,
			AlsoEmbedded2{4, 5}, 6}}
	buf.Reset()
	x.Marshal(buf)
	y := &HasEmbedded2{}
	y.Unmarshal(buf)
	if (x.A != y.A || x.B != y.B || x.C.X != y.C.X ||
		x.C.Z.X != y.C.Z.X || x.C.Z.Y != y.C.Z.Y ||
		x.C.Y != y.C.Y) {
		t.Fatalf("Structures not the same: ", x, y)
	}
}

func TestCached(t *testing.T) {
	buf.Reset()
	s.Marshal(buf)
	sc := &SlicedCache{}
	s2 := sc.Get()
	if s2 == nil {
		t.Fatalf("Eek - nil result from getting from object cache")
	}
	s2.Unmarshal(buf)
	sc.Put(s2)
	s3 := sc.Get()
	if s3 == nil {
		t.Fatalf("Eek - nil result on second get from object cache")
	}
	s3.Unmarshal(buf)
}
func BenchmarkReflectionMarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buf.Reset()
		binary.Write(buf, binary.LittleEndian, d)
	}
}

func BenchmarkGeneratedMarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buf.Reset()
		d.Marshal(buf)
	}
}

func BenchmarkGobMarshal(b *testing.B) {
	// Let's give gobs the benefit of the doubt here.
	enc := gob.NewEncoder(buf)
	buf.Reset()
	enc.Encode(d)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		enc.Encode(d)
	}
}


func BenchmarkReflectionUnmarshal(b *testing.B) {
	buf.Reset()
	d.Marshal(buf)
	by := buf.Bytes()
	buf2 := &bytes.Buffer{}
	for i := 0; i < b.N; i++ {
		buf2.Reset()
		buf2.Write(by)
		binary.Read(buf2, binary.LittleEndian, e)
	}
}

func BenchmarkGeneratedUnmarshal(b *testing.B) {
	buf.Reset()
	d.Marshal(buf)
	by := buf.Bytes()
	buf2 := &bytes.Buffer{}
	for i := 0; i < b.N; i++ {
		buf2.Reset()
		buf2.Write(by)
		e.Unmarshal(buf2)
	}
}

func BenchmarkGeneratedUnmarshalNew(b *testing.B) {
	buf.Reset()
	d.Marshal(buf)
	by := buf.Bytes()
	buf2 := &bytes.Buffer{}
	for i := 0; i < b.N; i++ {
		buf2.Reset()
		buf2.Write(by)
		e := &Demostruct{}
		e.Unmarshal(buf2)
	}
}

func BenchmarkGeneratedUnmarshalCached(b *testing.B) {
	c := &DemostructCache{}
	buf.Reset()
	d.Marshal(buf)
	by := buf.Bytes()
	buf2 := &bytes.Buffer{}
	for i := 0; i < b.N; i++ {
		buf2.Reset()
		buf2.Write(by)
		e := c.Get()
		e.Unmarshal(buf2)
		c.Put(e)
	}
}
