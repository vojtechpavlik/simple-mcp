package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestExecuteCommand_Templating(t *testing.T) {
	item := ContextItem{
		Name:    "Greet",
		Command: "echo Hello {{.name}}",
	}
	params := map[string]interface{}{
		"name": "World",
	}

	output, _, _, err := executeCommand(item, params, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.TrimSpace(output) != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", output)
	}
}

func TestExecuteCommand_Timeout(t *testing.T) {
	// This command sleeps for 2 seconds, but we set timeout to 1 second
	item := ContextItem{
		Name:           "Sleepy",
		Command:        "sleep 10",
		TimeoutSeconds: 1,
	}

	start := time.Now()
	_, _, _, err := executeCommand(item, nil, "")
	duration := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// We verify that it failed reasonably close to the timeout
	if duration.Seconds() > 5 {
		t.Errorf("test took too long (%v), timeout logic might be broken", duration)
	}
}

func TestExecuteCommand_InvalidTemplate(t *testing.T) {
	item := ContextItem{
		Command: "echo {{.missing_end_brace",
	}
	_, _, _, err := executeCommand(item, nil, "")
	if err == nil {
		t.Error("expected template parse error, got nil")
	}
}

func TestExecuteCommand_WorkingDirectory(t *testing.T) {
	item := ContextItem{
		Name:    "PrintWorkDir",
		Command: "pwd",
	}

	// Test with a specific directory
	output, _, _, err := executeCommand(item, nil, "/usr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.TrimSpace(output) != "/usr" {
		t.Errorf("expected '/usr', got '%s'", output)
	}

	// Test with the default /tmp directory
	output, _, _, err = executeCommand(item, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.TrimSpace(output) != "/tmp" {
		t.Errorf("expected '/tmp', got '%s'", output)
	}
}

func TestExecuteCommand_SecurityInjection(t *testing.T) {
	tempFile := "/tmp/simple-mcp-test-security"
	os.Remove(tempFile)
	defer os.Remove(tempFile)

	item := ContextItem{
		Name:    "Echo",
		Command: "echo {{.text}}",
	}

	testCases := []struct {
		name  string
		input string
	}{
		{"Semicolon", "dummy; touch " + tempFile},
		{"And", "dummy && touch " + tempFile},
		{"Pipe", "dummy | touch " + tempFile},
		{"Backticks", "dummy `touch " + tempFile + "`"},
		{"Subshell", "dummy $(touch " + tempFile + ")"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]interface{}{
				"text": tc.input,
			}
			_, _, _, _ = executeCommand(item, params, "")

			if _, err := os.Stat(tempFile); err == nil {
				t.Errorf("Security breach: file %s was created using %s injection", tempFile, tc.name)
				os.Remove(tempFile)
			}
		})
	}
}

func TestExecuteCommand_Quoting(t *testing.T) {
	itemUnquoted := ContextItem{
		Name:    "EchoUnquoted",
		Command: "printf '%s\n' {{.text}}",
	}
	itemQuoted := ContextItem{
		Name:    "EchoQuoted",
		Command: "printf '%s\n' \"{{.text}}\"",
	}

	params := map[string]interface{}{
		"text": "hello world",
	}

	// Unquoted: should split into two arguments
	output, _, _, _ := executeCommand(itemUnquoted, params, "")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines for unquoted space, got %d: %q", len(lines), output)
	}

	// Quoted: should stay as one argument
	output, _, _, _ = executeCommand(itemQuoted, params, "")
	lines = strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line for quoted space, got %d: %q", len(lines), output)
	}
}

func TestExecuteCommand_Globbing(t *testing.T) {
	// Create a few files to glob
	dir, _ := os.MkdirTemp("", "mcp-glob-test")
	defer os.RemoveAll(dir)
	os.WriteFile(dir+"/a", []byte("a"), 0644)
	os.WriteFile(dir+"/b", []byte("b"), 0644)

	itemUnquoted := ContextItem{
		Name:    "LsUnquoted",
		Command: "ls {{.pattern}}",
	}
	itemQuoted := ContextItem{
		Name:    "LsQuoted",
		Command: "ls \"{{.pattern}}\"",
	}

	params := map[string]interface{}{
		"pattern": dir + "/*",
	}

	// Unquoted: shell should expand the glob
	output, _, _, _ := executeCommand(itemUnquoted, params, "")
	if !strings.Contains(output, dir+"/a") || !strings.Contains(output, dir+"/b") {
		t.Errorf("expected glob expansion for unquoted, got: %q", output)
	}

	// Quoted: shell should NOT expand the glob (passing the literal '*' to ls, which should fail or just show the literal name)
	output, _, _, _ = executeCommand(itemQuoted, params, "")
	if strings.Contains(output, dir+"/a") && strings.Contains(output, dir+"/b") {
		t.Errorf("did NOT expect glob expansion for quoted, but got: %q", output)
	}
}

func TestExecuteCommand_Sanitization(t *testing.T) {
	tempFile := "/tmp/simple-mcp-test-sanitize"
	os.Remove(tempFile)
	defer os.Remove(tempFile)

	item := ContextItem{
		Name:    "Sanitize",
		Command: "echo {{index . \"bad-name; touch " + tempFile + "\"}}",
	}
	params := map[string]interface{}{
		"bad-name; touch " + tempFile: "safe value",
	}

	output, _, _, err := executeCommand(item, params, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.TrimSpace(output) != "safe value" {
		t.Errorf("expected 'safe value', got %q", output)
	}

	if _, err := os.Stat(tempFile); err == nil {
		t.Errorf("Security breach: file %s was created using parameter name injection", tempFile)
		os.Remove(tempFile)
	}
}
