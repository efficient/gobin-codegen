package encodedemo

type IsEmbedded struct {
	X int
	Y int
}

type HasEmbedded struct {
	A int
	B int
	C IsEmbedded
}
