// Copyright (c) 2025 Vojtech Pavlik <vojtech@suse.com>
//
// Created using AI tools
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package resource

import (
	"fmt"
	"log"
	"strings"

	"github.com/SUSE/simple-mcp/internal/config"
	"github.com/SUSE/simple-mcp/internal/executor"
)

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// GetResourceContent generates the content for a given resource, handling static content,
// dynamic command execution, and the combination of both.
func GetResourceContent(item config.ResourceItem, tmpDir string, verbose bool) (string, error) {
	var combinedContent strings.Builder

	// Append static content first
	if item.Content != "" {
		combinedContent.WriteString(item.Content)
	}

	// Then, append command output if a command is defined
	if item.Command != "" {
		cmdItem := config.ContextItem{Command: item.Command}
		output, err := executor.ExecuteCommand(cmdItem, nil, tmpDir)

		if err != nil {
			log.Printf("ERROR: Error executing command for resource %s (Exit Code: %d): %v", item.URI, output.ReturnCode, err)
			// Append error to content for visibility to the LLM
			combinedContent.WriteString(fmt.Sprintf("\nError executing command: %v. Output: %s", err, output.Result))
		} else {
			if verbose {
				log.Printf("Successfully executed command for resource %s, output: %d bytes, %d lines, exit code: %d, duration: %s", item.URI, len(output.Result), countLines(output.Result), output.ReturnCode, output.Duration)
			}
			combinedContent.WriteString(output.Result)
		}
	}

	return combinedContent.String(), nil
}
