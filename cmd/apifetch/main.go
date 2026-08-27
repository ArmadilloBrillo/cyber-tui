package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/config"
)

const defaultBaseURL = "https://api.cyberspace.online"

func main() {
	method := flag.String("method", "GET", "HTTP method")
	body := flag.String("body", "", "request body (JSON string)")
	baseURL := flag.String("base-url", "", "override API base URL")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: apifetch [--method METHOD] [--body JSON] [--base-url URL] <path>")
		os.Exit(1)
	}
	path := flag.Arg(0)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load config: %v\n", err)
		os.Exit(1)
	}
	if cfg.RefreshToken == "" {
		fmt.Fprintln(os.Stderr, "error: no saved session — log in to cyber-tui first")
		os.Exit(1)
	}

	base := defaultBaseURL
	if cfg.APIBaseURL != "" {
		base = cfg.APIBaseURL
	}
	if *baseURL != "" {
		base = *baseURL
	}

	client := api.NewHTTPClient(base)
	if _, err := client.LoginWithRefreshToken(cfg.RefreshToken); err != nil {
		fmt.Fprintf(os.Stderr, "error: refresh token: %v\n", err)
		os.Exit(1)
	}

	var bodyBytes []byte
	if *body != "" {
		bodyBytes = []byte(*body)
	}

	raw, err := client.RawRequest(*method, path, bodyBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if _, err := json.MarshalIndent(raw, "", "  "); err != nil {
		fmt.Fprintln(os.Stdout, string(raw))
		return
	}

	// MarshalIndent on a json.RawMessage wraps it in quotes; unmarshal first.
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		fmt.Fprintln(os.Stdout, string(raw))
		return
	}
	pretty, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprintln(os.Stdout, string(pretty))
}
