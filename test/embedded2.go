package encodedemo

type IsEmbedded2 struct {
	X int
    Z AlsoEmbedded2
	Y int
}

type AlsoEmbedded2 struct {
    X int
    Y int
}

type HasEmbedded2 struct {
	A int
	B int
	C IsEmbedded2
}
