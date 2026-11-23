package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"higress-mcp-instructions-demo/internal/demo"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	instructionsFile := flag.String("instructions-file", "/config/instructions.txt", "UTF-8 instructions file")
	flag.Parse()
	instructions, err := os.ReadFile(*instructionsFile)
	if err != nil {
		log.Fatal(err)
	}
	_, handler := demo.NewFixture(string(instructions))
	server := &http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("native SDK upstream listening on %s", *listen)
	log.Fatal(server.ListenAndServe())
}
