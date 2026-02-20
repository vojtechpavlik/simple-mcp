// Copyright (c) 2025 Vojtech Pavlik <vojtech@suse.com>
//
// Created using AI tools
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package shared

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// EmptyOutput is used when we return the result via *mcp.CallToolResult directly
type EmptyOutput struct{}

// NewTextResult creates a text result for a tool call
func NewTextResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: text,
			},
		},
	}
}

// NewErrorResult creates an error result for a tool call
func NewErrorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: text,
			},
		},
		IsError: true,
	}
}
