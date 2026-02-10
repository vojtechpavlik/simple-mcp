package executor

import (
	"os"
	"strings"
	"testing"

	"github.com/SUSE/simple-mcp/internal/config"
)

func TestExecuteCommand_FileScript_Template(t *testing.T) {
	// Create a temp script with a shebang and template variables
	f, err := os.CreateTemp("", "script-template-*.py")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	// Shell script to verify shebang and templating
	scriptContent := "#!/bin/sh\necho \"Hello {{.name}} from Shell\""
	if _, err := f.WriteString(scriptContent); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := os.Chmod(f.Name(), 0755); err != nil {
		t.Fatal(err)
	}

	item := config.ContextItem{
		Name:    "FileScriptTemplate",
		Command: f.Name(),
	}

	params := map[string]interface{}{
		"name": "Goose",
	}

	res, err := ExecuteCommand(item, params, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Hello Goose from Shell"
	if strings.TrimSpace(res.Result) != expected {
		t.Errorf("expected '%s', got '%s'", expected, res.Result)
	}
}

func TestExecuteCommand_FileScript_ComplexTemplate(t *testing.T) {
	// Create a temp script with a shebang and complex logic
	f, err := os.CreateTemp("", "script-complex-*.bash")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	scriptContent := "#!/bin/bash\nif [ \"{{.condition}}\" == \"true\" ]; then\n  echo \"Condition met\"\nelse\n  echo \"Condition failed\"\nfi"
	if _, err := f.WriteString(scriptContent); err != nil {
		t.Fatal(err)
	}
	f.Close()
	
	if err := os.Chmod(f.Name(), 0755); err != nil {
		t.Fatal(err)
	}

	item := config.ContextItem{
		Name:    "FileScriptComplex",
		Command: f.Name(),
	}

	// Test case 1: condition is true
	paramsTrue := map[string]interface{}{
		"condition": "true",
	}
	res, err := ExecuteCommand(item, paramsTrue, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(res.Result) != "Condition met" {
		t.Errorf("expected 'Condition met', got '%s'", res.Result)
	}

	// Test case 2: condition is false
	paramsFalse := map[string]interface{}{
		"condition": "false",
	}
	res, err = ExecuteCommand(item, paramsFalse, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(res.Result) != "Condition failed" {
		t.Errorf("expected 'Condition failed', got '%s'", res.Result)
	}
}
