// Copyright (c) 2025 Vojtech Pavlik <vojtech@suse.com>
//
// Created using AI tools
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolvePath(base, path string) (string, error) {
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
