package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"non24.app/server/internal/mcp"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	cfg := mcp.LoadConfigFromEnv()
	fmt.Fprintln(os.Stderr, cfg.StartupMessage())

	server := mcp.NewServer(cfg)
	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatalf("zeitboard MCP adapter stopped: %v", err)
	}
}
