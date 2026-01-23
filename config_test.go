package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	// Create a temporary config file
	content := `
apiVersion: v1
kind: DynamicContextSource
metadata:
  name: test-mcp
spec:
  contextItems:
    - name: TestTool
      command: echo test
      parameters: ["arg1"]
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Metadata.Name != "test-mcp" {
		t.Errorf("expected name test-mcp, got %s", cfg.Metadata.Name)
	}
	if len(cfg.Specification.Tools) != 1 {
		t.Errorf("expected 1 item, got %d", len(cfg.Specification.Tools))
	}
	if cfg.Specification.Tools[0].Name != "TestTool" {
		t.Errorf("expected tool TestTool, got %s", cfg.Specification.Tools[0].Name)
	}
}

func TestLoadConfig_Tools(t *testing.T) {
	// Create a temporary config file using 'tools' instead of 'contextItems'
	content := `
apiVersion: v1
kind: DynamicContextSource
metadata:
  name: test-mcp
spec:
  tools:
    - name: TestTool
      command: echo test
`
	tmpfile, err := os.CreateTemp("", "config-tools-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Specification.Tools) != 1 {
		t.Errorf("expected 1 item, got %d", len(cfg.Specification.Tools))
	}
	if cfg.Specification.Tools[0].Name != "TestTool" {
		t.Errorf("expected tool TestTool, got %s", cfg.Specification.Tools[0].Name)
	}
}

func TestLoadConfig_BothItemsAndTools(t *testing.T) {
	content := `
apiVersion: v1
kind: DynamicContextSource
metadata:
  name: test-mcp
spec:
  contextItems:
    - name: Tool1
      command: echo 1
  tools:
    - name: Tool2
      command: echo 2
`
	tmpfile, err := os.CreateTemp("", "config-both-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	tmpfile.Write([]byte(content))
	tmpfile.Close()

	_, err = LoadConfig(tmpfile.Name())
	if err == nil {
		t.Fatal("expected error when both contextItems and tools are present, got nil")
	}
	if !strings.Contains(err.Error(), "both 'contextItems' and 'tools' are defined") {
		t.Errorf("expected specific error message, got: %v", err)
	}
}

func TestLoadConfig_InvalidYaml(t *testing.T) {
	content := `
apiVersion: v1
metadata:
  name: broken
  - indentation_error: yes
`
	tmpfile, err := os.CreateTemp("", "bad-config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	tmpfile.Write([]byte(content))
	tmpfile.Close()

	_, err = LoadConfig(tmpfile.Name())
	if err == nil {
		t.Fatal("expected YAML parse error, got nil")
	}

	// Verify our custom error formatter is working (checking for line number info)
	if !strings.Contains(err.Error(), "line") {
		t.Errorf("expected error message to contain line number info, got: %s", err.Error())
	}
}

func TestLoadConfig_DefaultFile(t *testing.T) {
	filename := "simple-mcp.yaml"
	
	// Skip if the file is not found (e.g. running tests in isolation/different dir)
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Skipf("%s not found, skipping integration test", filename)
	}

	cfg, err := LoadConfig(filename)
	if err != nil {
		t.Fatalf("Failed to parse default config %s: %v", filename, err)
	}

	// Basic validation of the default config content
	expectedName := "simple-mcp"
	if cfg.Metadata.Name != expectedName {
		t.Errorf("Default config name mismatch. Expected '%s', got '%s'", expectedName, cfg.Metadata.Name)
	}

	if len(cfg.Specification.Resources) == 0 {
		t.Error("Default config should define at least one resource")
	}

	if len(cfg.Specification.Tools) == 0 {
		t.Error("Default config should define at least one tool")
	}

	// Verify that the contentFile was loaded correctly for the overview resource
	overviewResourceFound := false
	for _, resource := range cfg.Specification.Resources {
		if resource.URI == "simple-mcp://system/overview" {
			overviewResourceFound = true
			expectedContent := "This is a detailed overview of the system, loaded from an external file.\n"
			if resource.Content != expectedContent {
				t.Errorf("Expected overview resource content to be '%s', got '%s'", expectedContent, resource.Content)
			}
			break
		}
	}
	if !overviewResourceFound {
		t.Error("Did not find the 'simple-mcp://system/overview' resource in the default config")
	}
}

func TestLoadConfig_Options(t *testing.T) {
	content := `
apiVersion: v1
kind: DynamicContextSource
metadata:
  name: test-mcp
spec:
  listenAddr: ":9090"
  tmpDir: "/tmp/custom"
  verbose: true
  tools:
    - name: TestTool
      command: echo test
`
	tmpfile, err := os.CreateTemp("", "config-options-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Specification.ListenAddr != ":9090" {
		t.Errorf("expected listenAddr :9090, got %s", cfg.Specification.ListenAddr)
	}
	if cfg.Specification.TmpDir != "/tmp/custom" {
		t.Errorf("expected tmpDir /tmp/custom, got %s", cfg.Specification.TmpDir)
	}
	if cfg.Specification.Verbose == nil || *cfg.Specification.Verbose != true {
		t.Errorf("expected verbose true, got %v", cfg.Specification.Verbose)
	}
}

func TestResolveOptions(t *testing.T) {
	vTrue := true

	tests := []struct {
		name            string
		cfg             *Config
		cliListenAddr    string
		cliTmpDir        string
		cliVerbose       bool
		cliMaxAsyncTasks int
		setFlags         map[string]bool
		expectedAddr     string
		expectedTmp      string
		expectedVerbose  bool
		expectedMaxTasks int
	}{
		{
			name: "Defaults only",
			cfg: &Config{
				Specification: Spec{},
			},
			cliListenAddr:    "localhost:8080",
			cliTmpDir:        "",
			cliVerbose:       false,
			cliMaxAsyncTasks: 20,
			setFlags:         map[string]bool{},
			expectedAddr:     "localhost:8080",
			expectedTmp:      "",
			expectedVerbose:  false,
			expectedMaxTasks: 20,
		},
		{
			name: "Config overrides defaults",
			cfg: &Config{
				Specification: Spec{
					ListenAddr:    ":9090",
					TmpDir:        "/tmp/cfg",
					Verbose:       &vTrue,
					MaxAsyncTasks: 50,
				},
			},
			cliListenAddr:    "localhost:8080",
			cliTmpDir:        "",
			cliVerbose:       false,
			cliMaxAsyncTasks: 20,
			setFlags:         map[string]bool{},
			expectedAddr:     ":9090",
			expectedTmp:      "/tmp/cfg",
			expectedVerbose:  true,
			expectedMaxTasks: 50,
		},
		{
			name: "CLI overrides config",
			cfg: &Config{
				Specification: Spec{
					ListenAddr:    ":9090",
					TmpDir:        "/tmp/cfg",
					Verbose:       &vTrue,
					MaxAsyncTasks: 50,
				},
			},
			cliListenAddr:    ":7070",
			cliTmpDir:        "/tmp/cli",
			cliVerbose:       false,
			cliMaxAsyncTasks: 10,
			setFlags: map[string]bool{
				"listen-addr":     true,
				"tmpdir":          true,
				"verbose":         true,
				"max-async-tasks": true,
			},
			expectedAddr:     ":7070",
			expectedTmp:      "/tmp/cli",
			expectedVerbose:  false,
			expectedMaxTasks: 10,
		},
		{
			name: "Mixed override",
			cfg: &Config{
				Specification: Spec{
					ListenAddr:    ":9090",
					TmpDir:        "/tmp/cfg",
					Verbose:       &vTrue,
					MaxAsyncTasks: 50,
				},
			},
			cliListenAddr:    ":7070",
			cliTmpDir:        "",
			cliVerbose:       false,
			cliMaxAsyncTasks: 10,
			setFlags: map[string]bool{
				"listen-addr": true,
			},
			expectedAddr:     ":7070",
			expectedTmp:      "/tmp/cfg",
			expectedVerbose:  true,
			expectedMaxTasks: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, tmp, verb, maxTasks := resolveOptions(tt.cfg, tt.cliListenAddr, tt.cliTmpDir, tt.cliVerbose, tt.cliMaxAsyncTasks, tt.setFlags)
			if addr != tt.expectedAddr {
				t.Errorf("expected addr %s, got %s", tt.expectedAddr, addr)
			}
			if tmp != tt.expectedTmp {
				t.Errorf("expected tmp %s, got %s", tt.expectedTmp, tmp)
			}
			if verb != tt.expectedVerbose {
				t.Errorf("expected verbose %v, got %v", tt.expectedVerbose, verb)
			}
			if maxTasks != tt.expectedMaxTasks {
				t.Errorf("expected maxTasks %d, got %d", tt.expectedMaxTasks, maxTasks)
			}
		})
	}
}

func TestLoadConfig_UnifiedCore(t *testing.T) {
	filename := "unified-core.yaml"

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Skipf("%s not found, skipping integration test", filename)
	}

	cfg, err := LoadConfig(filename)
	if err != nil {
		t.Fatalf("Failed to parse %s: %v", filename, err)
	}

	if len(cfg.Specification.Tools) == 0 {
		t.Errorf("%s should define at least one tool", filename)
	}
}

func TestLoadConfig_DirectoryResource(t *testing.T) {
	// Create a temporary directory for the resource files
	resDir, err := os.MkdirTemp("", "res-dir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(resDir)

	// Create some files in the directory
	file1Path := filepath.Join(resDir, "file1.txt")
	if err := os.WriteFile(file1Path, []byte("content1"), 0644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(resDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	file2Path := filepath.Join(subDir, "file2.txt")
	if err := os.WriteFile(file2Path, []byte("content2"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a temporary config file referencing the directory
	content := fmt.Sprintf(`
apiVersion: v1
kind: DynamicContextSource
metadata:
  name: test-mcp
spec:
  resources:
    - uri: "simple-mcp://docs"
      description: "Test Docs"
      directory: "%s"
`, resDir)
	tmpfile, err := os.CreateTemp("", "config-dir-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Specification.Resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(cfg.Specification.Resources))
	}

	resourceMap := make(map[string]ResourceItem)
	for _, res := range cfg.Specification.Resources {
		resourceMap[res.URI] = res
	}

	if res, ok := resourceMap["simple-mcp://docs/file1.txt"]; !ok {
		t.Errorf("expected resource simple-mcp://docs/file1.txt not found")
	} else if res.Content != "content1" {
		t.Errorf("expected content1, got %s", res.Content)
	}

	if res, ok := resourceMap["simple-mcp://docs/subdir/file2.txt"]; !ok {
		t.Errorf("expected resource simple-mcp://docs/subdir/file2.txt not found")
	} else if res.Content != "content2" {
		t.Errorf("expected content2, got %s", res.Content)
	}
}

func TestLoadConfig_DirectoryResourceRelative(t *testing.T) {
	// Create a temporary directory for the config and resources
	baseDir, err := os.MkdirTemp("", "base-dir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	resSubDir := filepath.Join(baseDir, "docs")
	if err := os.Mkdir(resSubDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resSubDir, "info.txt"), []byte("info content"), 0644); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(baseDir, "config.yaml")
	content := `
apiVersion: v1
kind: DynamicContextSource
metadata:
  name: test-mcp
spec:
  resources:
    - uri: "simple-mcp://docs"
      description: "Test Docs"
      directory: "docs"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	found := false
	for _, res := range cfg.Specification.Resources {
		if res.URI == "simple-mcp://docs/info.txt" {
			found = true
			if res.Content != "info content" {
				t.Errorf("expected 'info content', got '%s'", res.Content)
			}
			break
		}
	}
	if !found {
		t.Error("relative directory resource not found")
	}
}

func TestLoadConfig_StructuredParameters(t *testing.T) {
	content := `
apiVersion: v1
kind: DynamicContextSource
metadata:
  name: test-mcp
spec:
  tools:
    - name: TestTool
      command: echo test
      parameters:
        - name: arg1
          type: integer
          validator: "^[0-9]+$"
        - arg2
`
	tmpfile, err := os.CreateTemp("", "config-struct-params-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Specification.Tools) != 1 {
		t.Fatalf("expected 1 item, got %d", len(cfg.Specification.Tools))
	}
	params := cfg.Specification.Tools[0].Parameters
	if len(params) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(params))
	}

	if params[0].Name != "arg1" || params[0].Type != "integer" || params[0].Validator != "^[0-9]+$" {
		t.Errorf("parameter 0 mismatch: %+v", params[0])
	}

	if params[1].Name != "arg2" || params[1].Type != "" || params[1].Validator != "" {
		t.Errorf("parameter 1 mismatch: %+v", params[1])
	}
}
