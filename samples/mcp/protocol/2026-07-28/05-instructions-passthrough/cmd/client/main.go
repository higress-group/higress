package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"higress-mcp-instructions-demo/internal/demo"
)

func main() {
	endpoint := flag.String("endpoint", "", "native MCP endpoint")
	adminURL := flag.String("admin-url", "http://127.0.0.1:18083", "upstream health/ledger base URL")
	file := flag.String("instructions-file", "fixture/instructions.txt", "exact UTF-8 instructions expected at upstream")
	absent := flag.Bool("instructions-absent", false, "require an empty SDK instructions value")
	serverName := flag.String("server-name", demo.GatewayName, "expected SDK discovery serverInfo name")
	methods := flag.String("expected-methods", "server/discover,tools/list,tools/call", "exact upstream method sequence")
	phase := flag.String("phase", "manual", "request correlation phase")
	runID := flag.String("run-id", "", "unique verification run ID (random by default)")
	flag.Parse()
	if *endpoint == "" {
		log.Fatal("--endpoint is required")
	}
	instructions, err := os.ReadFile(*file)
	if err != nil {
		log.Fatal(err)
	}
	expected := string(instructions)
	if *absent {
		expected = ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	report, err := demo.Run(ctx, demo.Options{Endpoint: *endpoint, AdminURL: *adminURL,
		ExpectedInstructions: expected, UpstreamInstructions: string(instructions), ServerName: *serverName,
		Phase: *phase, RunID: *runID, ExpectedMethods: strings.Split(*methods, ",")})
	if err != nil {
		log.Fatal(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		log.Fatal(err)
	}
}
