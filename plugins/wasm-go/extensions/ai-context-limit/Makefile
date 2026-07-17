.PHONY: prepare build build-go clean

prepare:
	@sh ./prepare.sh

build: build-go

build-go: prepare
	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm .

clean:
	rm -f main.wasm
	rm -f bpe/o200k_base.tiktoken
