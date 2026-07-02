// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var exitURL = flag.String("url", "http://127.0.0.1:18082/api/lifecycle/exit", "POST /api/lifecycle/exit on curator control")

// START OMIT
func main() {
	flag.Parse()
	if err := postCuratorExit(*exitURL); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// END OMIT

func postCuratorExit(url string) error {
	body, err := json.Marshal(map[string]string{"reason": "demo presentation"})
	if err != nil {
		return fmt.Errorf("encode body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	return printExitResponse(resp)
}

func printExitResponse(resp *http.Response) error {
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	fmt.Printf("HTTP %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))
	if len(payload) > 0 {
		fmt.Println(string(payload))
	}
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("want %d Accepted", http.StatusAccepted)
	}
	return nil
}
