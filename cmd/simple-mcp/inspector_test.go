// Copyright (c) 2025 Vojtech Pavlik <vojtech@suse.com>
//
// Created using AI tools
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestInspector(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not found, skipping inspector tests")
	}

	// Get project root
	cwd, _ := filepath.Abs(".")
	rootDir := filepath.Dir(filepath.Dir(cwd))
	serverBin := filepath.Join(rootDir, "simple-mcp")

	// Ensure the server is built
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = rootDir
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build server: %v", err)
	}

	t.Run("TransportStdio", func(t *testing.T) {
		runInspectorTest(t, rootDir, []string{"--cli", "--method", "tools/list", serverBin, "-t", "stdio"})
		runInspectorTest(t, rootDir, []string{"--cli", "--method", "resources/list", serverBin, "-t", "stdio"})
	})

	t.Run("TransportSSE", func(t *testing.T) {
		port := getFreePort(t)
		addr := fmt.Sprintf("localhost:%d", port)

		serverCmd := exec.Command(serverBin, "-t", "sse", "--listen-addr", addr)
		serverCmd.Dir = rootDir
		if err := serverCmd.Start(); err != nil {
			t.Fatalf("failed to start server: %v", err)
		}
		defer serverCmd.Process.Kill()

		waitForServer(t, addr)

		serverURL := fmt.Sprintf("http://%s/mcp", addr)
		runInspectorTest(t, rootDir, []string{"--cli", "--method", "tools/list", "--transport", "sse", serverURL})
		runInspectorTest(t, rootDir, []string{"--cli", "--method", "resources/list", "--transport", "sse", serverURL})
	})

	t.Run("TransportHTTP", func(t *testing.T) {
		port := getFreePort(t)
		addr := fmt.Sprintf("localhost:%d", port)

		serverCmd := exec.Command(serverBin, "-t", "http", "--listen-addr", addr)
		serverCmd.Dir = rootDir
		if err := serverCmd.Start(); err != nil {
			t.Fatalf("failed to start server: %v", err)
		}
		defer serverCmd.Process.Kill()

		waitForServer(t, addr)

		serverURL := fmt.Sprintf("http://%s/mcp", addr)
		runInspectorTest(t, rootDir, []string{"--cli", "--method", "tools/list", "--transport", "http", serverURL})
		runInspectorTest(t, rootDir, []string{"--cli", "--method", "resources/list", "--transport", "http", serverURL})
	})
}

func runInspectorTest(t *testing.T, dir string, args []string) {
	t.Helper()
	// Use npx with --yes to avoid prompts
	fullArgs := append([]string{"--yes", "@modelcontextprotocol/inspector"}, args...)
	cmd := exec.Command("npx", fullArgs...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("inspector failed: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
	}

	// Verify output is valid JSON
	var result interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse inspector output as JSON: %v\nOutput: %s", err, stdout.String())
	}
}

func getFreePort(t *testing.T) int {
	t.Helper()
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to resolve address: %v", err)
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	tick := time.Tick(200 * time.Millisecond)
	for {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for server at %s", addr)
		case <-tick:
			conn, err := net.Dial("tcp", addr)
			if err == nil {
				conn.Close()
				return
			}
		}
	}
}
