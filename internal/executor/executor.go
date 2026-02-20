// Copyright (c) 2025 Vojtech Pavlik <vojtech@suse.com>
//
// Created using AI tools
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package main provides the command execution logic for the server.
// It handles secure template rendering of commands and their execution with
// strictly enforced timeouts.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/SUSE/simple-mcp/internal/config"
)

type ToolResult struct {
	Result     string
	Duration   time.Duration
	ReturnCode int
}

// ExecuteCommand renders the command template with the provided parameters
// and executes it in a shell. It returns the combined stdout/stderr,
// the exit code, and any Go-level error that occurred.
func ExecuteCommand(item config.ContextItem, params map[string]interface{}, workDir string) (ToolResult, error) {
	startTime := time.Now()

	// We separate code from data by passing parameters as environment variables.
	envVars := make([]string, 0, len(params))
	templateData := make(map[string]string)

	for key, value := range params {
		// Sanitize the key to be a valid shell variable name
		var sanitizedKey strings.Builder
		for _, r := range key {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
				sanitizedKey.WriteRune(r)
			} else {
				sanitizedKey.WriteRune('_')
			}
		}

		envVarName := fmt.Sprintf("_MCP_VAR_%s", sanitizedKey.String())
		strValue := fmt.Sprintf("%v", value)
		envVars = append(envVars, fmt.Sprintf("%s=%s", envVarName, strValue))
		templateData[key] = "${" + envVarName + "}"
	}
	var templateText string
	interpreter := "sh"
	interpreterArgs := []string{}

	// check if command is a file
	isScriptFile := false
	hasShebang := false
	if strings.HasPrefix(item.Command, "/") {
		if info, err := os.Stat(item.Command); err == nil && !info.IsDir() {
			if content, err := os.ReadFile(item.Command); err == nil {
				isScriptFile = true
				templateText = string(content)
				// Check for shebang
				firstLine, _, _ := strings.Cut(templateText, "\n")
				if strings.HasPrefix(firstLine, "#!") {
					hasShebang = true
					shebang := strings.TrimSpace(strings.TrimPrefix(firstLine, "#!"))
					parts := strings.Fields(shebang)
					if len(parts) > 0 {
						candidate := parts[0]
						if _, err := os.Stat(candidate); err == nil {
							interpreter = candidate
							interpreterArgs = []string{}
							if len(parts) > 1 {
								interpreterArgs = append(interpreterArgs, parts[1:]...)
							}
						}
					}
				}
			}
		}
	}
	if !isScriptFile {
		interpreterArgs = []string{"-c"}
		header := "set -o pipefail\n"
		if item.Header != "" {
			header = item.Header
		}
		footer := ""
		if item.Footer != "" {
			footer = item.Footer
		}
		templateText = header + item.Command + footer
	}
	// Parse the command template
	tmpl, err := template.New("command").Parse(templateText)
	if err != nil {
		return ToolResult{}, fmt.Errorf("invalid command template in config: %w", err)
	}

	// Render the command string using the variable references
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		return ToolResult{}, fmt.Errorf("failed to build command from template: %w", err)
	}
	finalCommand := buf.String()

	if item.TimeoutSeconds <= 0 {
		item.TimeoutSeconds = 30
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(item.TimeoutSeconds)*time.Second)
	defer cancel()

	// We use exec.Command (without Context) so we can manually manage the
	// wait/kill cycle. This prevents 'CombinedOutput' from hanging on
	// open pipes even after the context is done.
	var cmd *exec.Cmd
	if isScriptFile && hasShebang {
		tmpFile, err := os.CreateTemp("", "mcp-script-*")
		if err != nil {
			return ToolResult{}, fmt.Errorf("failed to create temp file: %w", err)
		}
		tempPath := tmpFile.Name()
		defer os.Remove(tempPath)

		if _, err := tmpFile.WriteString(finalCommand); err != nil {
			tmpFile.Close()
			return ToolResult{}, fmt.Errorf("failed to write temp file: %w", err)
		}
		tmpFile.Close()

		if err := os.Chmod(tempPath, 0755); err != nil {
			return ToolResult{}, fmt.Errorf("failed to chmod temp file: %w", err)
		}
		cmd = exec.Command(tempPath)
	} else {
		cmd = exec.Command(interpreter, append(interpreterArgs, finalCommand)...)
	}

	// Assign the process to a new Process Group so we can identify all children.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Start with our safe parameter variables
	cmd.Env = envVars

	// Now add the os env the command should get
	for _, envVarName := range item.EnvVars {
		cmd.Env = append(cmd.Env, os.Getenv(envVarName))
	}

	// Set the working directory for the command.
	if workDir != "" {
		cmd.Dir = workDir
	} else {
		cmd.Dir = "/tmp"
	}

	// Capture both stdout and stderr in the same buffer (mimics CombinedOutput)
	var outputBuf bytes.Buffer
	cmd.Stdout = &outputBuf
	cmd.Stderr = &outputBuf

	// Start the command
	if err := cmd.Start(); err != nil {
		return ToolResult{}, fmt.Errorf("failed to start command: %w", err)
	}

	// Create a channel to signal command completion
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		close(done)
	}()

	// Wait for either the command to finish or the timeout
	select {
	case <-ctx.Done():
		// Timeout occurred!
		// Kill the entire process group to ensure children (like 'sleep') are killed.
		// We ignore errors here because the process might already be gone.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)

		return ToolResult{
			Duration:   time.Since(startTime),
			ReturnCode: -1,
		}, fmt.Errorf("command timed out after %d seconds", item.TimeoutSeconds)

	case err := <-done:
		// Command finished naturally
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		if exitCode != 0 {
			ignored := false
			for _, code := range item.IgnoreExitCodes {
				if code == exitCode {
					ignored = true
					break
				}
			}

			if !ignored {
				return ToolResult{
					Duration:   time.Since(startTime),
					ReturnCode: exitCode,
					Result:     outputBuf.String(),
				}, fmt.Errorf("command failed: %w", err)
			}
		}
		return ToolResult{
			Duration:   time.Since(startTime),
			ReturnCode: exitCode,
			Result:     strings.TrimSpace(outputBuf.String()),
		}, nil
	}
}
