.DEFAULT:
build:
 	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o ./main.wasm .
	mv main.wasm ../../../../docker-compose-test/