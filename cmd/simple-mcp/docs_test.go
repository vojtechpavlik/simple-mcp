// Copyright (c) 2025 Vojtech Pavlik <vojtech@suse.com>
//
// Created using AI tools
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"os/exec"
	"testing"
)

func TestManPages(t *testing.T) {
	tools := []string{"mandoc"}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found, skipping manpage linting", tool)
		}
	}

	manpages := []string{"../../simple-mcp.1", "../../simple-mcp-cli.1"}
	for _, mp := range manpages {
		cmd := exec.Command("mandoc", "-Tlint", mp)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("mandoc lint failed for %s: %v\nOutput: %s", mp, err, string(output))
		}
	}
}

func TestMarkdown(t *testing.T) {
	tools := []string{"markdownlint-cli2"}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found, skipping markdown linting", tool)
		}
	}

	cmd := exec.Command("markdownlint-cli2", "../../README.md", "../../llm-config-generation/README.md", "../../llm-config-generation/system_prompt_mcp_architect.md")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("markdownlint failed: %v\nOutput: %s", err, string(output))
	}
}
