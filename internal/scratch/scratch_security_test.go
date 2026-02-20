// Copyright (c) 2025 Vojtech Pavlik <vojtech@suse.com>
//
// Created using AI tools
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package scratch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScratchSymlinkSecurity(t *testing.T) {
	// Create a real scratch space
	tmpDir, err := os.MkdirTemp("", "scratch-security-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Resolve tmpDir to its real path (in case /tmp is a symlink)
	realTmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)
	realTmpDir, err = filepath.Abs(realTmpDir)
	require.NoError(t, err)

	// Create a file OUTSIDE the scratch space
	outsideDir, err := os.MkdirTemp("", "outside-scratch-")
	require.NoError(t, err)
	defer os.RemoveAll(outsideDir)

	outsideFile := filepath.Join(outsideDir, "secret.txt")
	err = os.WriteFile(outsideFile, []byte("sensitive info"), 0644)
	require.NoError(t, err)

	t.Run("SymlinkToOutsideFile", func(t *testing.T) {
		linkPath := filepath.Join(realTmpDir, "link_to_secret")
		err := os.Symlink(outsideFile, linkPath)
		require.NoError(t, err)

		// Attempt to read via the link
		res, err := readFile(realTmpDir, "link_to_secret")
		require.NoError(t, err)

		// If the vulnerability exists, this will succeed and return the content
		// We WANT it to fail or return an error result
		if !res.IsError {
			t.Errorf("Security breach: successfully read file outside scratch space via symlink. Content: %s", res.Content[0])
		}
	})

	t.Run("SymlinkToOutsideDir", func(t *testing.T) {
		linkPath := filepath.Join(realTmpDir, "link_to_outside")
		err := os.Symlink(outsideDir, linkPath)
		require.NoError(t, err)

		// Attempt to create a file in the outside dir via the link
		res, err := createFile(realTmpDir, "link_to_outside/new_file.txt", "pwned")
		require.NoError(t, err)

		if !res.IsError {
			t.Errorf("Security breach: successfully created file outside scratch space via symlink.")
			// Check if it was actually created
			if _, err := os.Stat(filepath.Join(outsideDir, "new_file.txt")); err == nil {
				t.Errorf("Verified: file was created at %s", filepath.Join(outsideDir, "new_file.txt"))
			}
		}
	})

	t.Run("NestedSymlinkToOutside", func(t *testing.T) {
		subdir := filepath.Join(realTmpDir, "subdir")
		err := os.Mkdir(subdir, 0755)
		require.NoError(t, err)

		linkPath := filepath.Join(subdir, "link_to_outside")
		err = os.Symlink(outsideDir, linkPath)
		require.NoError(t, err)

		res, err := readFile(realTmpDir, "subdir/link_to_outside/secret.txt")
		require.NoError(t, err)

		if !res.IsError {
			t.Errorf("Security breach: successfully read file outside scratch space via nested symlink.")
		}
	})

	t.Run("BrokenSymlinkToOutside", func(t *testing.T) {
		// A symlink that points to a non-existent file outside
		brokenPath := filepath.Join(outsideDir, "non-existent.txt")
		linkPath := filepath.Join(realTmpDir, "broken_link")
		err := os.Symlink(brokenPath, linkPath)
		require.NoError(t, err)

		// Attempt to CREATE the file via the broken link
		res, err := createFile(realTmpDir, "broken_link", "pwned")
		require.NoError(t, err)

		if !res.IsError {
			t.Errorf("Security breach: successfully created file outside scratch space via broken symlink.")
			// Check if it was actually created
			if _, err := os.Stat(brokenPath); err == nil {
				t.Errorf("Verified: file was created at %s", brokenPath)
				os.Remove(brokenPath)
			}
		}
	})

	t.Run("InternalSymlinkAllowed", func(t *testing.T) {
		// Create a file in the scratch space
		innerDir := filepath.Join(realTmpDir, "inner")
		err := os.Mkdir(innerDir, 0755)
		require.NoError(t, err)

		targetFile := filepath.Join(innerDir, "target.txt")
		err = os.WriteFile(targetFile, []byte("internal content"), 0644)
		require.NoError(t, err)

		// Create a symlink pointing to it
		linkPath := filepath.Join(realTmpDir, "link_to_inner")
		err = os.Symlink(innerDir, linkPath)
		require.NoError(t, err)

		// Attempt to read via the link
		res, err := readFile(realTmpDir, "link_to_inner/target.txt")
		require.NoError(t, err)

		assert.False(t, res.IsError, "Should be able to read internal symlink")
		assert.Equal(t, "internal content", res.Content[0].(*mcp.TextContent).Text)
	})

	t.Run("DoubleDotFilenameAllowed", func(t *testing.T) {
		res, err := createFile(realTmpDir, "..hidden.txt", "hidden content")
		require.NoError(t, err)
		assert.False(t, res.IsError, "Should be able to create file starting with ..")

		res, err = readFile(realTmpDir, "..hidden.txt")
		require.NoError(t, err)
		assert.Equal(t, "hidden content", res.Content[0].(*mcp.TextContent).Text)
	})
}
