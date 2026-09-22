BINARY := mkcr

.PHONY: build clean

build:
	go build -o $(BINARY) .

clean:
	rm -f $(BINARY)
	go build -o $(BINARY) .
