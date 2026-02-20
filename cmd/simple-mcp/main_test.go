package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SUSE/simple-mcp/internal/config"
)

func TestSearchResources(t *testing.T) {
	resourceMap := map[string]config.ResourceItem{
		"test://apple": {
			URI:         "test://apple",
			Description: "A red fruit",
			Content:     "Static content about apples.",
		},
		"test://banana": {
			URI:         "test://banana",
			Description: "A yellow fruit",
			Content:     "Static content about bananas.",
		},
		"test://cherry": {
			URI:         "test://cherry",
			Description: "A small red fruit",
			Content:     "Static content about cherries.",
		},
	}

	tests := []struct {
		name          string
		query         string
		expectedFound int
		contains      []string
		notContains   []string
		expectError   bool
	}{
		{
			name:          "Search by URI",
			query:         "apple",
			expectedFound: 1,
			contains:      []string{"test://apple"},
			notContains:   []string{"test://banana", "test://cherry"},
		},
		{
			name:          "Search by Description",
			query:         "yellow",
			expectedFound: 1,
			contains:      []string{"test://banana"},
		},
		{
			name:          "Search by Content",
			query:         "cherries",
			expectedFound: 1,
			contains:      []string{"test://cherry"},
		},
		{
			name:          "Search multiple",
			query:         "red",
			expectedFound: 2,
			contains:      []string{"test://apple", "test://cherry"},
			notContains:   []string{"test://banana"},
		},
		{
			name:          "Complex Regex (alternation)",
			query:         "apple|banana",
			expectedFound: 2,
			contains:      []string{"test://apple", "test://banana"},
			notContains:   []string{"test://cherry"},
		},
		{
			name:          "Complex Regex (character class and wildcards)",
			query:         "r[e-i]d",
			expectedFound: 2,
			contains:      []string{"test://apple", "test://cherry"},
		},
		{
			name:          "No results",
			query:         "durian",
			expectedFound: 0,
			contains:      []string{"No resources matched"},
		},
		{
			name:        "Invalid regex",
			query:       "[",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := searchResources(resourceMap, tt.query)
			if (err != nil) != tt.expectError {
				t.Errorf("searchResources() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if tt.expectError {
				return
			}

			if tt.expectedFound > 0 {
				expectedFoundStr := fmt.Sprintf("Found %d matching resources", tt.expectedFound)
				if !strings.Contains(result, expectedFoundStr) {
					t.Errorf("result expected to contain %q, but didn't. Result: %q", expectedFoundStr, result)
				}
			}

			for _, c := range tt.contains {
				if !strings.Contains(result, c) {
					t.Errorf("result expected to contain %q, but didn't. Result: %q", c, result)
				}
			}
			for _, nc := range tt.notContains {
				if strings.Contains(result, nc) {
					t.Errorf("result expected NOT to contain %q, but did. Result: %q", nc, result)
				}
			}
		})
	}
}

func TestCheckShellSafe(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"safe_path", true},
		{"safe/path/to/file.txt", true},
		{"path with space", false},
		{"path\\ with\\ space", true},
		{"path;evil", false},
		{"path\\;evil", true},
		{"path&evil", false},
		{"path&&evil", false},
		{"path\\&\\&evil", true},
		{"$(whoami)", false},
		{"\\$(whoami)", false},
		{"\\$\\(\\whoami\\)", true},
		{"`whoami`", false},
		{"\\`whoami\\`", true},
		{"\\`whoami\\\\\\`", true},
		{"trailing\\", false},
		{"escaped\\\\", true},
		{"escaped\\\\ ", false},
		{"escaped\\\\\\ ", true},
	}

	for _, tt := range tests {
		err := checkShellSafe(tt.input)
		if (err == nil) != tt.valid {
			t.Errorf("checkShellSafe(%q) valid = %v, error = %v", tt.input, tt.valid, err)
		}
	}
}

func TestValidateParameter(t *testing.T) {
	// Setup a temporary directory for tmpDir tests
	tmpDir, err := os.MkdirTemp("", "validate-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a file and dir in tmpDir
	os.Mkdir(filepath.Join(tmpDir, "mysubdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "myfile.txt"), []byte("test"), 0644)

	tests := []struct {
		name    string
		param   config.Parameter
		value   string
		valid   bool
		wantErr string
	}{
		{
			name:  "Integer Valid",
			param: config.Parameter{Name: "p", Type: "integer"},
			value: "123",
			valid: true,
		},
		{
			name:    "Integer Invalid",
			param:   config.Parameter{Name: "p", Type: "integer"},
			value:   "abc",
			valid:   false,
			wantErr: "must be a valid integer",
		},
		{
			name:  "Number Valid",
			param: config.Parameter{Name: "p", Type: "number"},
			value: "12.34",
			valid: true,
		},
		{
			name:  "Word Valid",
			param: config.Parameter{Name: "p", Type: "word"},
			value: "HelloWorld123",
			valid: true,
		},
		{
			name:    "Word Invalid",
			param:   config.Parameter{Name: "p", Type: "word"},
			value:   "Hello World",
			valid:   false,
			wantErr: "must be an alphanumeric word without spaces",
		},
		{
			name:  "Filename Valid",
			param: config.Parameter{Name: "p", Type: "filename"},
			value: "my_file.txt",
			valid: true,
		},
		{
			name:    "Filename Invalid (path separator)",
			param:   config.Parameter{Name: "p", Type: "filename"},
			value:   "dir/file.txt",
			valid:   false,
			wantErr: "must not contain path separators",
		},
		{
			name:  "Regexp Valid",
			param: config.Parameter{Name: "p", Validator: "^[a-z]+$"},
			value: "abc",
			valid: true,
		},
		{
			name:    "Regexp Invalid",
			param:   config.Parameter{Name: "p", Validator: "^[a-z]+$"},
			value:   "123",
			valid:   false,
			wantErr: "failed regexp check",
		},
		{
			name:  "tmpFile Valid",
			param: config.Parameter{Name: "p", Type: "tmpFile"},
			value: "myfile.txt",
			valid: true,
		},
		{
			name:    "tmpFile Invalid (is dir)",
			param:   config.Parameter{Name: "p", Type: "tmpFile"},
			value:   "mysubdir",
			valid:   false,
			wantErr: "path is a directory in scratch space",
		},
		{
			name:  "tmpDir Valid",
			param: config.Parameter{Name: "p", Type: "tmpDir"},
			value: "mysubdir",
			valid: true,
		},
		{
			name:    "tmpDir Invalid (security)",
			param:   config.Parameter{Name: "p", Type: "tmpDir"},
			value:   "../",
			valid:   false,
			wantErr: "path must not contain '..'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateParameter(tt.param, tt.value, tmpDir)
			if (err == nil) != tt.valid {
				t.Errorf("validateParameter() error = %v, valid %v", err, tt.valid)
			}
			if !tt.valid && tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
