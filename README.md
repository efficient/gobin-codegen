This is a work-in-progress stub generator for marshaling/unmarshaling go structs into the encoding/binary format. It handles many basic types, but if you get fancy, you'll break it. It does not handle strings.

The reason this code exists, for a simple, statically-sized struct:

```
BenchmarkReflectionMarshal       1000000              2016 ns/op
BenchmarkGeneratedMarshal       10000000               223 ns/op
```

Operation: Takes a file with go structs as input:

```go
package duck

type Quack struct {
        X int
        Y int
}
```

Run as:

```sh
bin/bi duck_decl.go > duck_marshal.go
```
and outputs go code that does the same thing that binary.Write would do, but faster:

```go
package duck

func (t *Quack) Marshal(w io.Writer) {
        var b [16]byte
        binary.LittleEndian.PutUint64(b[0:8], uint64(t.X))
        binary.LittleEndian.PutUint64(b[8:16], uint64(t.Y))
        w.Write(b[:])
}
```
You can use these stubs in your own code:

```go
    q := &Quack{X: 1, Y: 2}
    buf := new bytes.Buffer
    if err := q.Marshal(buf) {
      fmt.Println("Could not marshal data: ", err)
      // handle error appropriately, return, etc.
    }
    fmt.Println("Marshaled data: ", buf)

    q2 := &Quack{}
    if err := q2.Unmarshal(buf) {
      fmt.Println("Could not unmarshal buf: ", err)
      // handle error here
    }
    fmt.Println("q2: ", q2)
```
In addition to standard encoding/binary formats, gobin-codegen will output code to handle variable-length slice data within structs if you ask it to. It does so by first encoding the length of the slice as a varint, and then writing the members of the slice. Such a struct is not compatible with the standard encoding/binary, but will work if you know both sides use gobin-codegen. In this way, gobin-codegen can marshal []byte and other variable length data types.

This is not production-quality code. Its optimizations are limited to small completely static structs - otherwise, the code it outputs will be both longer and slower than that shown above, but probably still 4x faster than the reflection-based marshaling.
Generates code to handle marshaling and unmarshaling to/from 
encoding/binary.

It probably isn't nearly as general as you'd like it to be.
Don't depend on it working properly.  Don't think that it
handles corner cases well.  You get the idea. :)

NOTE:  This library emits code for variable-length slices
by prefixing them with a varint length.  This is a departure
from the encoding/binary format, which cannot handle variable
length slices.  If you use a slice type, your binary output
will not be compatible with stock encoding/binary any more.

use:
```sh
  export GOPATH=`/bin/pwd`
  go install bi
  bin/bi /path/to/your/go/struct/decl.go > generated_code.go
```

  use generated_code.go as you see fit.
