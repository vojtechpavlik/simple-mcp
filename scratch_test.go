// Copyright (c) 2025 Vojtech Pavlik <vojtech@suse.com>
//
// Created using AI tools
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScratchLogic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scratch-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("CreateDirectory", func(t *testing.T) {
		res, err := createDirectory(tmpDir, "test-dir")
		require.NoError(t, err)
		assert.Equal(t, "Directory created successfully.", res.Content[0].(*mcp.TextContent).Text)
		_, err = os.Stat(filepath.Join(tmpDir, "test-dir"))
		assert.NoError(t, err)
	})

	t.Run("CreateFile", func(t *testing.T) {
		res, err := createFile(tmpDir, "test-file.txt", "hello world\n")
		require.NoError(t, err)
		assert.Equal(t, "File created successfully.", res.Content[0].(*mcp.TextContent).Text)
		content, err := os.ReadFile(filepath.Join(tmpDir, "test-file.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "hello world\n", string(content))
	})

	t.Run("CreateFile_WithSubdir", func(t *testing.T) {
		res, err := createFile(tmpDir, "subdir/test-file.txt", "hello subdir\n")
		require.NoError(t, err)
		assert.Equal(t, "File created successfully.", res.Content[0].(*mcp.TextContent).Text)
		content, err := os.ReadFile(filepath.Join(tmpDir, "subdir/test-file.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "hello subdir\n", string(content))
	})

	t.Run("CopyResourceToFile", func(t *testing.T) {
		resourceMap := map[string]ResourceItem{
			"simple-mcp://content": {
				URI:     "simple-mcp://content",
				Content: "resource content",
			},
			"simple-mcp://command": {
				URI:     "simple-mcp://command",
				Command: "echo command content",
			},
		}

		res, err := copyResourceToFile(resourceMap, tmpDir, false, "simple-mcp://content", "resource-file.txt")
		require.NoError(t, err)
		assert.Equal(t, "File created successfully.", res.Content[0].(*mcp.TextContent).Text)
		content, err := os.ReadFile(filepath.Join(tmpDir, "resource-file.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "resource content", string(content))

		res, err = copyResourceToFile(resourceMap, tmpDir, false, "simple-mcp://command", "command-file.txt")
		require.NoError(t, err)
		assert.Equal(t, "File created successfully.", res.Content[0].(*mcp.TextContent).Text)
		content, err = os.ReadFile(filepath.Join(tmpDir, "command-file.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "command content", string(content))
	})

	t.Run("CopyResourceToFile_Combined", func(t *testing.T) {
		resourceMap := map[string]ResourceItem{
			"simple-mcp://combined": {
				URI:     "simple-mcp://combined",
				Content: "static content\n",
				Command: "echo dynamic content",
			},
		}

		res, err := copyResourceToFile(resourceMap, tmpDir, false, "simple-mcp://combined", "combined-file.txt")
		require.NoError(t, err)
		assert.Equal(t, "File created successfully.", res.Content[0].(*mcp.TextContent).Text)
		content, err := os.ReadFile(filepath.Join(tmpDir, "combined-file.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "static content\ndynamic content", string(content))
	})

	t.Run("CopyResourceTree", func(t *testing.T) {
		resourceMap := map[string]ResourceItem{
			"prefix://a/file1.txt": {URI: "prefix://a/file1.txt", Content: "content1"},
			"prefix://a/b/file2.txt": {URI: "prefix://a/b/file2.txt", Content: "content2"},
			"prefix://other/file3.txt": {URI: "prefix://other/file3.txt", Content: "content3"},
			"prefix://a": {URI: "prefix://a", Content: "contentA"},
		}

		t.Run("MatchWithSlash", func(t *testing.T) {
			res, err := copyResourceTree(resourceMap, tmpDir, false, "prefix://a/", "tree-slash")
			require.NoError(t, err)
			assert.Contains(t, res.Content[0].(*mcp.TextContent).Text, "Successfully copied 2 resources")

			content1, _ := os.ReadFile(filepath.Join(tmpDir, "tree-slash/file1.txt"))
			assert.Equal(t, "content1", string(content1))
			content2, _ := os.ReadFile(filepath.Join(tmpDir, "tree-slash/b/file2.txt"))
			assert.Equal(t, "content2", string(content2))
		})

		t.Run("MatchWithoutSlash", func(t *testing.T) {
			// Should match prefix://a, prefix://a/file1.txt, prefix://a/b/file2.txt
			// Wait, if I copy all of them to "tree-no-slash":
			// prefix://a -> tree-no-slash
			// prefix://a/file1.txt -> tree-no-slash/file1.txt
			// This should fail if tree-no-slash is created as a file first.
			// The order is random in map iteration.
			// Let's use a cleaner example where they don't overlap as file/dir.

			resourceMapClean := map[string]ResourceItem{
				"prefix://a/file1.txt": {URI: "prefix://a/file1.txt", Content: "content1"},
				"prefix://a/b/file2.txt": {URI: "prefix://a/b/file2.txt", Content: "content2"},
			}
			res, err := copyResourceTree(resourceMapClean, tmpDir, false, "prefix://a", "tree-no-slash")
			require.NoError(t, err)
			assert.Contains(t, res.Content[0].(*mcp.TextContent).Text, "Successfully copied 2 resources")

			content1, _ := os.ReadFile(filepath.Join(tmpDir, "tree-no-slash/file1.txt"))
			assert.Equal(t, "content1", string(content1))
			content2, _ := os.ReadFile(filepath.Join(tmpDir, "tree-no-slash/b/file2.txt"))
			assert.Equal(t, "content2", string(content2))
		})

		t.Run("NoMatch", func(t *testing.T) {
			res, err := copyResourceTree(resourceMap, tmpDir, false, "prefix://nonexistent", "tree-none")
			require.NoError(t, err)
			assert.True(t, res.IsError)
			assert.Contains(t, res.Content[0].(*mcp.TextContent).Text, "no resources found")
		})

		t.Run("PartialMatchSecurity", func(t *testing.T) {
			// prefix://a should NOT match prefix://ab/file.txt
			resourceMapPartial := map[string]ResourceItem{
				"prefix://ab/file.txt": {URI: "prefix://ab/file.txt", Content: "content"},
			}
			res, err := copyResourceTree(resourceMapPartial, tmpDir, false, "prefix://a", "tree-partial")
			require.NoError(t, err)
			assert.True(t, res.IsError)
		})

		t.Run("Overwrite", func(t *testing.T) {
			_, err := createFile(tmpDir, "tree-overwrite/file1.txt", "old content")
			require.NoError(t, err)

			resourceMapOverwrite := map[string]ResourceItem{
				"prefix://a/file1.txt": {URI: "prefix://a/file1.txt", Content: "new content"},
			}
			res, err := copyResourceTree(resourceMapOverwrite, tmpDir, false, "prefix://a/", "tree-overwrite")
			require.NoError(t, err)
			assert.False(t, res.IsError)

			content1, _ := os.ReadFile(filepath.Join(tmpDir, "tree-overwrite/file1.txt"))
			assert.Equal(t, "new content", string(content1))
		})
	})

	t.Run("ReadFile", func(t *testing.T) {
		_, err := createFile(tmpDir, "test-file-for-read.txt", "hello read\n")
		require.NoError(t, err)
		res, err := readFile(tmpDir, "test-file-for-read.txt")
		require.NoError(t, err)
		assert.Equal(t, "hello read\n", res.Content[0].(*mcp.TextContent).Text)
	})

	t.Run("ReplaceInFile", func(t *testing.T) {
		_, err := createFile(tmpDir, "test-file-for-replace.txt", "hello world\nhello gopher\n")
		require.NoError(t, err)

		t.Run("ReplaceFirst", func(t *testing.T) {
			res, err := replaceInFile(tmpDir, "test-file-for-replace.txt", "hello", "hi", false)
			require.NoError(t, err)
			assert.Equal(t, "File modified successfully.", res.Content[0].(*mcp.TextContent).Text)
			content, _ := os.ReadFile(filepath.Join(tmpDir, "test-file-for-replace.txt"))
			assert.Equal(t, "hi world\nhello gopher\n", string(content))
		})

		t.Run("ReplaceAll", func(t *testing.T) {
			// Reset content
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test-file-for-replace.txt"), []byte("hello world\nhello gopher\n"), 0644))
			res, err := replaceInFile(tmpDir, "test-file-for-replace.txt", "hello", "hi", true)
			require.NoError(t, err)
			assert.Equal(t, "File modified successfully.", res.Content[0].(*mcp.TextContent).Text)
			content, _ := os.ReadFile(filepath.Join(tmpDir, "test-file-for-replace.txt"))
			assert.Equal(t, "hi world\nhi gopher\n", string(content))
		})

		t.Run("CaptureGroups", func(t *testing.T) {
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test-file-for-replace.txt"), []byte("version: 1.2.3\n"), 0644))
			_, err := replaceInFile(tmpDir, "test-file-for-replace.txt", `version: (\d+\.\d+\.\d+)`, "v$1", false)
			require.NoError(t, err)
			content, _ := os.ReadFile(filepath.Join(tmpDir, "test-file-for-replace.txt"))
			assert.Equal(t, "v1.2.3\n", string(content))
		})

		t.Run("MultiLine", func(t *testing.T) {
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test-file-for-replace.txt"), []byte("start\nmiddle\nend\n"), 0644))
			_, err := replaceInFile(tmpDir, "test-file-for-replace.txt", `start.*end`, "done", false)
			require.NoError(t, err)
			content, _ := os.ReadFile(filepath.Join(tmpDir, "test-file-for-replace.txt"))
			assert.Equal(t, "done\n", string(content))
		})

		t.Run("PatternNotFound", func(t *testing.T) {
			res, err := replaceInFile(tmpDir, "test-file-for-replace.txt", "nonexistent", "replacement", false)
			require.NoError(t, err)
			assert.True(t, res.IsError)
			assert.Contains(t, res.Content[0].(*mcp.TextContent).Text, "pattern not found")
		})

		t.Run("InvalidRegex", func(t *testing.T) {
			res, err := replaceInFile(tmpDir, "test-file-for-replace.txt", "[unclosed", "replacement", false)
			require.NoError(t, err)
			assert.True(t, res.IsError)
			assert.Contains(t, res.Content[0].(*mcp.TextContent).Text, "invalid regular expression")
		})
	})

	t.Run("ListDirectory", func(t *testing.T) {
		listDir := filepath.Join(tmpDir, "list-test")
		require.NoError(t, os.Mkdir(listDir, 0755))
		_, err := createFile(listDir, "file1.txt", "content1\n")
		require.NoError(t, err)
		_, err = createDirectory(listDir, "subdir")
		require.NoError(t, err)

		res, err := listDirectory(tmpDir, "list-test")
		require.NoError(t, err)

		expectedContent := "file1.txt\nsubdir/\n"
		assert.Equal(t, expectedContent, res.Content[0].(*mcp.TextContent).Text)
	})

	t.Run("DeleteFile", func(t *testing.T) {
		_, err := createFile(tmpDir, "test-file-for-delete.txt", "content\n")
		require.NoError(t, err)
		res, err := deleteFile(tmpDir, "test-file-for-delete.txt")
		require.NoError(t, err)
		assert.Equal(t, "File deleted successfully.", res.Content[0].(*mcp.TextContent).Text)
		_, err = os.Stat(filepath.Join(tmpDir, "test-file-for-delete.txt"))
		assert.Error(t, err)
	})

	t.Run("RemoveDirectory", func(t *testing.T) {
		_, err := createDirectory(tmpDir, "dir-for-remove")
		require.NoError(t, err)
		res, err := removeDirectory(tmpDir, "dir-for-remove")
		require.NoError(t, err)
		assert.Equal(t, "Directory removed successfully.", res.Content[0].(*mcp.TextContent).Text)
		_, err = os.Stat(filepath.Join(tmpDir, "dir-for-remove"))
		assert.Error(t, err)
	})

	t.Run("PathSecurity", func(t *testing.T) {
		paths := []string{
			"/etc/passwd",
			"../",
			"test/../../",
		}
		for _, path := range paths {
			t.Run(path, func(t *testing.T) {
				_, err := resolvePath(tmpDir, path)
				assert.Error(t, err)
			})
		}
	})
}
