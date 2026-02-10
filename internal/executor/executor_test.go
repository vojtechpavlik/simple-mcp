package executor

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SUSE/simple-mcp/internal/config"
)

func TestExecuteCommand_Templating(t *testing.T) {
	item := config.ContextItem{
		Name:    "Greet",
		Command: "echo Hello {{.name}}",
	}
	params := map[string]interface{}{
		"name": "World",
	}

	res, err := ExecuteCommand(item, params, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.TrimSpace(res.Result) != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", res.Result)
	}
}


func TestExecuteCommand_Timeout(t *testing.T) {
	// This command sleeps for 2 seconds, but we set timeout to 1 second
	item := config.ContextItem{
		Name:           "Sleepy",
		Command:        "sleep 2",
		TimeoutSeconds: 1,
	}

	start := time.Now()
	_, err := ExecuteCommand(item, nil, "")
	duration := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// We verify that it failed reasonably close to the timeout
	if duration.Seconds() > 1.8 {
		t.Errorf("test took too long (%v), timeout logic might be broken", duration)
	}
}

func TestExecuteCommand_InvalidTemplate(t *testing.T) {
	item := config.ContextItem{
		Command: "echo {{.missing_end_brace",
	}
	_, err := ExecuteCommand(item, nil, "")
	if err == nil {
		t.Error("expected template parse error, got nil")
	}
}

func TestExecuteCommand_WorkingDirectory(t *testing.T) {
	item := config.ContextItem{
		Name:    "PrintWorkDir",
		Command: "pwd",
	}

	// Test with a specific directory
	res, err := ExecuteCommand(item, nil, "/usr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.TrimSpace(res.Result) != "/usr" {
		t.Errorf("expected '/usr', got '%s'", res.Result)
	}

	// Test with the default /tmp directory
	res, err = ExecuteCommand(item, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.TrimSpace(res.Result) != "/tmp" {
		t.Errorf("expected '/tmp', got '%s'", res.Result)
	}
}

func TestExecuteCommand_SecurityInjection(t *testing.T) {
	tempFile := "/tmp/simple-mcp-test-security"
	os.Remove(tempFile)
	defer os.Remove(tempFile)

	item := config.ContextItem{
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
			_, _ = ExecuteCommand(item, params, "")

			if _, err := os.Stat(tempFile); err == nil {
				t.Errorf("Security breach: file %s was created using %s injection", tempFile, tc.name)
				os.Remove(tempFile)
			}
		})
	}
}

func TestExecuteCommand_Quoting(t *testing.T) {
	itemUnquoted := config.ContextItem{
		Name:    "EchoUnquoted",
		Command: "printf '%s\n' {{.text}}",
	}
	itemQuoted := config.ContextItem{
		Name:    "EchoQuoted",
		Command: "printf '%s\n' \"{{.text}}\"",
	}

	params := map[string]interface{}{
		"text": "hello world",
	}

	// Unquoted: should split into two arguments
	res, _ := ExecuteCommand(itemUnquoted, params, "")
	lines := strings.Split(strings.TrimSpace(res.Result), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines for unquoted space, got %d: %q", len(lines), res.Result)
	}

	// Quoted: should stay as one argument
	res, _ = ExecuteCommand(itemQuoted, params, "")
	lines = strings.Split(strings.TrimSpace(res.Result), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line for quoted space, got %d: %q", len(lines), res.Result)
	}
}

func TestExecuteCommand_Globbing(t *testing.T) {
	// Create a few files to glob
	dir, _ := os.MkdirTemp("", "mcp-glob-test")
	defer os.RemoveAll(dir)
	os.WriteFile(dir+"/a", []byte("a"), 0644)
	os.WriteFile(dir+"/b", []byte("b"), 0644)

	itemUnquoted := config.ContextItem{
		Name:    "LsUnquoted",
		Command: "ls {{.pattern}}",
	}
	itemQuoted := config.ContextItem{
		Name:    "LsQuoted",
		Command: "ls \"{{.pattern}}\"",
	}

	params := map[string]interface{}{
		"pattern": dir + "/*",
	}

	// Unquoted: shell should expand the glob
	res, _ := ExecuteCommand(itemUnquoted, params, "")
	if !strings.Contains(res.Result, dir+"/a") || !strings.Contains(res.Result, dir+"/b") {
		t.Errorf("expected glob expansion for unquoted, got: %q", res.Result)
	}

	// Quoted: shell should NOT expand the glob (passing the literal '*' to ls, which should fail or just show the literal name)
	res, _ = ExecuteCommand(itemQuoted, params, "")
	if strings.Contains(res.Result, dir+"/a") && strings.Contains(res.Result, dir+"/b") {
		t.Errorf("did NOT expect glob expansion for quoted, but got: %q", res.Result)
	}
}

func TestExecuteCommand_Sanitization(t *testing.T) {
	tempFile := "/tmp/simple-mcp-test-sanitize"
	os.Remove(tempFile)
	defer os.Remove(tempFile)

	item := config.ContextItem{
		Name:    "Sanitize",
		Command: "echo {{index . \"bad-name; touch " + tempFile + "\"}}",
	}
	params := map[string]interface{}{
		"bad-name; touch " + tempFile: "safe value",
	}

	res, err := ExecuteCommand(item, params, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.TrimSpace(res.Result) != "safe value" {
		t.Errorf("expected 'safe value', got %q", res.Result)
	}

	if _, err := os.Stat(tempFile); err == nil {
		t.Errorf("Security breach: file %s was created using parameter name injection", tempFile)
		os.Remove(tempFile)
	}
}

func TestExecuteCommand_FileScript(t *testing.T) {
	// Create a temp script
	f, err := os.CreateTemp("", "script-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	scriptContent := "#!/bin/sh\necho Hello File"
	if _, err := f.WriteString(scriptContent); err != nil {
		t.Fatal(err)
	}
	f.Close()

	item := config.ContextItem{
		Name:    "FileScript",
		Command: f.Name(),
	}

	res, err := ExecuteCommand(item, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.TrimSpace(res.Result) != "Hello File" {
		t.Errorf("expected 'Hello File', got '%s'", res.Result)
	}
}

func TestExecuteCommand_FileScriptWithShebangArgs(t *testing.T) {
	// Create a temp script with a shebang that has arguments
	// We use /usr/bin/env sh to test the argument parsing logic
	f, err := os.CreateTemp("", "script-shebang-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	scriptContent := "#!/usr/bin/env sh\necho Hello Shebang"
	if _, err := f.WriteString(scriptContent); err != nil {
		t.Fatal(err)
	}
	f.Close()

	item := config.ContextItem{
		Name:    "FileScriptShebang",
		Command: f.Name(),
	}

	res, err := ExecuteCommand(item, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.TrimSpace(res.Result) != "Hello Shebang" {
		t.Errorf("expected 'Hello Shebang', got '%s'", res.Result)
	}
}

func TestExecuteCommand_IgnoreExitCodes(t *testing.T) {
	itemIgnored := config.ContextItem{
		Name:            "Exit1",
		Command:         "exit 1",
		IgnoreExitCodes: []int{1},
	}

	res, err := ExecuteCommand(itemIgnored, nil, "")
	if err != nil {
		t.Errorf("expected nil error for ignored exit code, got %v", err)
	}

	if res.ReturnCode != 1 {
		t.Errorf("expected return code 1, got %d", res.ReturnCode)
	}

	itemNotIgnored := config.ContextItem{
		Name:            "Exit2",
		Command:         "exit 2",
		IgnoreExitCodes: []int{1},
	}
	_, err = ExecuteCommand(itemNotIgnored, nil, "")
	if err == nil {
		t.Error("expected error for non-ignored exit code 2, got nil")
	}
}
