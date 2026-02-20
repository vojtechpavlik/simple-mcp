package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListTools(t *testing.T) {
	// Get the project root directory
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	rootDir := filepath.Dir(filepath.Dir(dir))
	t.Logf("start test in: %s", dir)
	// Build the server and client from the project root
	buildCmdClean := exec.Command("make", "clean")
	buildCmdClean.Dir = rootDir
	err = buildCmdClean.Run()
	if err != nil {
		t.Fatalf("Failed to clean up: %v", err)
	}
	t.Logf("cleaned up: %s", buildCmdClean.String())
	buildCmd := exec.Command("make")
	buildCmd.Dir = rootDir
	err = buildCmd.Run()
	if err != nil {
		t.Fatalf("Failed to build server and client: %v", err)
	}
	t.Logf("built client and server: %s", buildCmd.String())
	port := os.Getenv("SIMPLE_MCP_PORT")
	if port == "" {
		port = "8080"
	}
	// Start the server from the project root
	serverCmd := exec.Command("./simple-mcp", "--listen-addr", "localhost:"+port)
	serverCmd.Dir = rootDir
	var serverOut bytes.Buffer
	serverCmd.Stdout = &serverOut
	serverCmd.Stderr = &serverOut
	err = serverCmd.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v out: %s", err, serverOut.String())
	}
	defer serverCmd.Process.Kill()
	t.Logf("started server ./simple-mcp")

	// Give the server a moment to start up
	time.Sleep(3 * time.Second)

	// Run the client from the project root and capture the output
	clientCmd := exec.Command("./simple-mcp-cli", "--server", "localhost:"+port, "list-tools")
	clientCmd.Dir = rootDir
	var clientOut bytes.Buffer
	clientCmd.Stdout = &clientOut
	clientCmd.Stderr = &clientOut
	err = clientCmd.Run()
	if err != nil {
		t.Fatalf("Client command failed: %v out: %s server: %s", err, clientOut.String(), serverOut.String())
	}

	// Verify that the client output contains the expected tool names
	expectedTools := []string{"ListPendingTasks", "TaskStatus", "ListResources", "GetResource"}
	output := clientOut.String()
	for _, tool := range expectedTools {
		if !strings.Contains(output, tool) {
			t.Errorf("Expected to find tool '%s' in the client output, but it was not found.", tool)
		}
	}
}
