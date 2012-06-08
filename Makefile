all : src/bi/bi

clean:
	rm src/bi/bi

src/bi/bi: src/bi/bi.go src/binidl/binidl.go
	go tool fix $^
	go tool vet $^
	gofmt -s -w $^
	(cd src ; GOPATH=$$PWD/.. go build -o bi/bi bi/bi.go)
