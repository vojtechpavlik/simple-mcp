// Copyright (c) 2025 Vojtech Pavlik <vojtech@suse.com>
//
// Created using AI tools
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"regexp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Input structs for scratch tools
type CreateFileRequest struct {
	Path    string `json:"path" jsonschema:"The path to the file within the scratch space."`
	Content string `json:"content" jsonschema:"The content of the file. Do not forget to include a newline character on the last line of a text file."`
}

type ReadFileRequest struct {
	Path string `json:"path" jsonschema:"The path to the file within the scratch space."`
}

type DeleteFileRequest struct {
	Path string `json:"path" jsonschema:"The path to the file within the scratch space."`
}

type ReplaceInFileRequest struct {
	Path        string `json:"path" jsonschema:"The path to the file within the scratch space."`
	Pattern     string `json:"pattern" jsonschema:"The regular expression pattern to search for."`
	Replacement string `json:"replacement" jsonschema:"The replacement string. Supports capture groups (e.g., $1)."`
	ReplaceAll  bool   `json:"replaceAll,omitempty" jsonschema:"description=If true, replace all occurrences. If false (default), replace only the first occurrence."`
}

type ListDirectoryRequest struct {
	Path string `json:"path" jsonschema:"The path to the directory within the scratch space. Absolute paths are not allowed."`
}

type CreateDirectoryRequest struct {
	Path string `json:"path" jsonschema:"The path to the directory within the scratch space."`
}

type RemoveDirectoryRequest struct {
	Path string `json:"path" jsonschema:"The path to the directory within the scratch space."`
}

type CopyResourceToFileRequest struct {
	ResourceURI string `json:"resourceURI" jsonschema:"The URI of the resource to copy."`
	Path        string `json:"path" jsonschema:"The path to the destination file within the scratch space."`
}

type CopyResourceTreeRequest struct {
	ResourcePrefix  string `json:"resourcePrefix" jsonschema:"The prefix of the resource URIs to copy."`
	DestinationPath string `json:"destinationPath" jsonschema:"The destination directory path within the scratch space."`
}

// EmptyOutput is used when we return the result via *mcp.CallToolResult directly
type EmptyOutput struct{}

// Helper to create a text result
func newTextResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: text,
			},
		},
	}
}

// Helper to create an error result
func newErrorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: text,
			},
		},
		IsError: true,
	}
}

// registerScratchTools registers the file and directory manipulation tools.
func registerScratchTools(mcpServer *mcp.Server, resourceMap map[string]ResourceItem, tmpDir string, verbose bool) {
	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "CreateFile", Description: "Creates a new file in the scratch space."},
		func(ctx context.Context, request *mcp.CallToolRequest, req CreateFileRequest) (*mcp.CallToolResult, EmptyOutput, error) {
			if verbose {
				log.Printf("Handling CreateFile request for path: %s", req.Path)
			}
			res, err := createFile(tmpDir, req.Path, req.Content)
			return res, EmptyOutput{}, err
		})

	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "ReadFile", Description: "Reads the content of a file in the scratch space."},
		func(ctx context.Context, request *mcp.CallToolRequest, req ReadFileRequest) (*mcp.CallToolResult, EmptyOutput, error) {
			if verbose {
				log.Printf("Handling ReadFile request for path: %s", req.Path)
			}
			res, err := readFile(tmpDir, req.Path)
			return res, EmptyOutput{}, err
		})

	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "DeleteFile", Description: "Deletes a file in the scratch space."},
		func(ctx context.Context, request *mcp.CallToolRequest, req DeleteFileRequest) (*mcp.CallToolResult, EmptyOutput, error) {
			if verbose {
				log.Printf("Handling DeleteFile request for path: %s", req.Path)
			}
			res, err := deleteFile(tmpDir, req.Path)
			return res, EmptyOutput{}, err
		})

	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "ReplaceInFile", Description: "Replaces a pattern in a file in the scratch space using a regular expression."},
		func(ctx context.Context, request *mcp.CallToolRequest, req ReplaceInFileRequest) (*mcp.CallToolResult, EmptyOutput, error) {
			if verbose {
				log.Printf("Handling ReplaceInFile request for path: %s", req.Path)
			}
			res, err := replaceInFile(tmpDir, req.Path, req.Pattern, req.Replacement, req.ReplaceAll)
			return res, EmptyOutput{}, err
		})

	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "ListDirectory", Description: "Lists the contents of a directory in the scratch space."},
		func(ctx context.Context, request *mcp.CallToolRequest, req ListDirectoryRequest) (*mcp.CallToolResult, EmptyOutput, error) {
			if verbose {
				log.Printf("Handling ListDirectory request for path: %s", req.Path)
			}
			res, err := listDirectory(tmpDir, req.Path)
			return res, EmptyOutput{}, err
		})

	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "CreateDirectory", Description: "Creates a new directory in the scratch space."},
		func(ctx context.Context, request *mcp.CallToolRequest, req CreateDirectoryRequest) (*mcp.CallToolResult, EmptyOutput, error) {
			if verbose {
				log.Printf("Handling CreateDirectory request for path: %s", req.Path)
			}
			res, err := createDirectory(tmpDir, req.Path)
			return res, EmptyOutput{}, err
		})

	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "RemoveDirectory", Description: "Removes an empty directory in the scratch space."},
		func(ctx context.Context, request *mcp.CallToolRequest, req RemoveDirectoryRequest) (*mcp.CallToolResult, EmptyOutput, error) {
			if verbose {
				log.Printf("Handling RemoveDirectory request for path: %s", req.Path)
			}
			res, err := removeDirectory(tmpDir, req.Path)
			return res, EmptyOutput{}, err
		})

	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "CopyResourceToFile", Description: "Copies the content of a resource to a file in the scratch space."},
		func(ctx context.Context, request *mcp.CallToolRequest, req CopyResourceToFileRequest) (*mcp.CallToolResult, EmptyOutput, error) {
			if verbose {
				log.Printf("Handling CopyResourceToFile request for resourceURI: %s, path: %s", req.ResourceURI, req.Path)
			}
			res, err := copyResourceToFile(resourceMap, tmpDir, verbose, req.ResourceURI, req.Path)
			return res, EmptyOutput{}, err
		})

	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "CopyResourceTree", Description: "Recursively copies all resources whose URIs start with a given prefix into a directory in the scratch space."},
		func(ctx context.Context, request *mcp.CallToolRequest, req CopyResourceTreeRequest) (*mcp.CallToolResult, EmptyOutput, error) {
			if verbose {
				log.Printf("Handling CopyResourceTree request for resourcePrefix: %s, destinationPath: %s", req.ResourcePrefix, req.DestinationPath)
			}
			res, err := copyResourceTree(resourceMap, tmpDir, verbose, req.ResourcePrefix, req.DestinationPath)
			return res, EmptyOutput{}, err
		})
}

func copyResourceToFile(resourceMap map[string]ResourceItem, tmpDir string, verbose bool, resourceURI, path string) (*mcp.CallToolResult, error) {
	item, ok := resourceMap[resourceURI]
	if !ok {
		return newErrorResult(fmt.Sprintf("resource not found: %s", resourceURI)), nil
	}

	content, err := getResourceContent(item, tmpDir, verbose)
	if err != nil {
		return newErrorResult(fmt.Sprintf("failed to get resource content for %s: %v", resourceURI, err)), nil
	}

	if content == "" {
		return newErrorResult(fmt.Sprintf("resource %s has no content or command", resourceURI)), nil
	}

	return createFile(tmpDir, path, content)
}

func copyResourceTree(resourceMap map[string]ResourceItem, tmpDir string, verbose bool, resourcePrefix, destinationPath string) (*mcp.CallToolResult, error) {
	var matchedURIs []string
	for uri := range resourceMap {
		if uri == resourcePrefix {
			matchedURIs = append(matchedURIs, uri)
			continue
		}
		if strings.HasPrefix(uri, resourcePrefix) {
			rest := uri[len(resourcePrefix):]
			if strings.HasPrefix(rest, "/") || strings.HasSuffix(resourcePrefix, "/") {
				matchedURIs = append(matchedURIs, uri)
			}
		}
	}

	if len(matchedURIs) == 0 {
		return newErrorResult(fmt.Sprintf("no resources found matching prefix: %s", resourcePrefix)), nil
	}

	for _, uri := range matchedURIs {
		item := resourceMap[uri]
		relPath := strings.TrimPrefix(uri, resourcePrefix)
		relPath = strings.TrimPrefix(relPath, "/")

		targetPath := destinationPath
		if relPath != "" {
			targetPath = filepath.Join(destinationPath, relPath)
		}

		content, err := getResourceContent(item, tmpDir, verbose)
		if err != nil {
			return newErrorResult(fmt.Sprintf("failed to get resource content for %s: %v", uri, err)), nil
		}

		if content == "" {
			return newErrorResult(fmt.Sprintf("resource %s has no content or command", uri)), nil
		}

		res, err := createFile(tmpDir, targetPath, content)
		if err != nil {
			return nil, err
		}
		if res.IsError {
			return res, nil
		}
	}

	return newTextResult(fmt.Sprintf("Successfully copied %d resources to %s.", len(matchedURIs), destinationPath)), nil
}

func resolvePath(base, path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	cleanedPath := filepath.Clean(path)
	for _, part := range strings.Split(filepath.ToSlash(cleanedPath), "/") {
		if part == ".." {
			return "", fmt.Errorf("path must not contain '..'")
		}
	}

	parts := strings.Split(filepath.ToSlash(cleanedPath), "/")
	current := base
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}

		next := filepath.Join(current, part)

		// Evaluate symlinks of the next component.
		resolved, err := filepath.EvalSymlinks(next)
		if err == nil {
			current = resolved
		} else if os.IsNotExist(err) {
			// If it doesn't exist, it might be a broken symlink or just a missing file/dir.
			if info, lerr := os.Lstat(next); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
				// It's a broken symlink! We must check where it points.
				target, rerr := os.Readlink(next)
				if rerr != nil {
					return "", fmt.Errorf("could not read broken symlink: %v", rerr)
				}
				if !filepath.IsAbs(target) {
					target = filepath.Join(current, target)
				}
				current = filepath.Clean(target)
			} else {
				// It's just a non-existent component.
				current = next
			}
		} else {
			// Some other error (e.g. permission)
			return "", fmt.Errorf("could not resolve path component %s: %v", part, err)
		}

		// Verify we are still within base after each component.
		rel, err := filepath.Rel(base, current)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("path escapes the scratch directory")
		}
	}

	return current, nil
}

func createFile(tmpDir, path, content string) (*mcp.CallToolResult, error) {
	fullPath, err := resolvePath(tmpDir, path)
	if err != nil {
		return newErrorResult(err.Error()), nil
	}
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return newErrorResult(fmt.Sprintf("failed to create parent directories: %v", err)), nil
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return newErrorResult(fmt.Sprintf("failed to create file: %v", err)), nil
	}
	return newTextResult("File created successfully."), nil
}

func readFile(tmpDir, path string) (*mcp.CallToolResult, error) {
	fullPath, err := resolvePath(tmpDir, path)
	if err != nil {
		return newErrorResult(err.Error()), nil
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return newErrorResult(fmt.Sprintf("failed to read file: %v", err)), nil
	}
	return newTextResult(string(content)), nil
}

func deleteFile(tmpDir, path string) (*mcp.CallToolResult, error) {
	fullPath, err := resolvePath(tmpDir, path)
	if err != nil {
		return newErrorResult(err.Error()), nil
	}
	if err := os.Remove(fullPath); err != nil {
		return newErrorResult(fmt.Sprintf("failed to delete file: %v", err)), nil
	}
	return newTextResult("File deleted successfully."), nil
}

func replaceInFile(tmpDir, path, pattern, replacement string, replaceAll bool) (*mcp.CallToolResult, error) {
	fullPath, err := resolvePath(tmpDir, path)
	if err != nil {
		return newErrorResult(err.Error()), nil
	}
	contentBytes, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return newErrorResult(fmt.Sprintf("failed to read file: %v", err)), nil
	}
	content := string(contentBytes)

	re, err := regexp.Compile("(?s)" + pattern)
	if err != nil {
		return newErrorResult(fmt.Sprintf("invalid regular expression: %v", err)), nil
	}

	var newContent string
	if replaceAll {
		if !re.MatchString(content) {
			return newErrorResult("pattern not found in file"), nil
		}
		newContent = re.ReplaceAllString(content, replacement)
	} else {
		indices := re.FindStringSubmatchIndex(content)
		if indices == nil {
			return newErrorResult("pattern not found in file"), nil
		}
		result := []byte{}
		result = re.ExpandString(result, replacement, content, indices)
		newContent = content[:indices[0]] + string(result) + content[indices[1]:]
	}

	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return newErrorResult(fmt.Sprintf("failed to write modified file: %v", err)), nil
	}
	return newTextResult("File modified successfully."), nil
}

func listDirectory(tmpDir, path string) (*mcp.CallToolResult, error) {
	fullPath, err := resolvePath(tmpDir, path)
	if err != nil {
		return newErrorResult(err.Error()), nil
	}
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return newErrorResult(fmt.Sprintf("failed to list directory: %v", err)), nil
	}
	var out strings.Builder
	for _, entry := range entries {
		if entry.IsDir() {
			fmt.Fprintf(&out, "%s/\n", entry.Name())
		} else {
			fmt.Fprintf(&out, "%s\n", entry.Name())
		}
	}
	return newTextResult(out.String()), nil
}

func createDirectory(tmpDir, path string) (*mcp.CallToolResult, error) {
	fullPath, err := resolvePath(tmpDir, path)
	if err != nil {
		return newErrorResult(err.Error()), nil
	}
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return newErrorResult(fmt.Sprintf("failed to create directory: %v", err)), nil
	}
	return newTextResult("Directory created successfully."), nil
}

func removeDirectory(tmpDir, path string) (*mcp.CallToolResult, error) {
	fullPath, err := resolvePath(tmpDir, path)
	if err != nil {
		return newErrorResult(err.Error()), nil
	}
	if err := os.Remove(fullPath); err != nil {
		return newErrorResult(fmt.Sprintf("failed to remove directory: %v", err)), nil
	}
	return newTextResult("Directory removed successfully."), nil
}
