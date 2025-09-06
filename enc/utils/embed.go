package utils

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	
	"enc/internal/logger"
)

// ExtractEmbedDotPrefix extracts srcDir (inside the embedded FS efs) into destDir.
// For entries under srcDir, the top-level directory names are prefixed with a '.' in destDir.
// If destDir is empty, the current working directory is used.
func ExtractEmbedDotPrefix(efs fs.FS, srcDir, destDir string) error {
	if destDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		destDir = cwd
	}

	// Normalize srcDir: remove trailing separator if present
	srcDir = filepath.Clean(srcDir)

	return fs.WalkDir(efs, srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Compute relative path under srcDir
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		// skip the root itself
		if rel == "." {
			return nil
		}

		// Guard: prevent unexpected path traversal
		if strings.HasPrefix(rel, "..") {
			return fs.ErrInvalid
		}

		// split top-level component
		var top, rest string
		if idx := strings.IndexByte(rel, byte(os.PathSeparator)); idx >= 0 {
			top = rel[:idx]
			rest = rel[idx+1:]
		} else {
			top = rel
			rest = ""
		}

		// target path: top-level directories are prefixed with "."
		var target string
		if rest == "" {
			// top-level entry (either file or dir)
			if d.IsDir() {
				target = filepath.Join(destDir, "."+top)
			} else {
				// top-level file — do not prefix (change this if you want to prefix files too)
				target = filepath.Join(destDir, top)
			}
		} else {
			// deeper entry under a top-level directory; prefix the top dir
			target = filepath.Join(destDir, "."+top, rest)
		}

		if d.IsDir() {
			// create directory
			return os.MkdirAll(target, 0o755)
		}

		// ensure parent exists
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		// copy file contents
		in, err := efs.Open(path)
		if err != nil {
			return err
		}
		defer func(err := in.Close(); if err != nil {
			logger.Error(err)
		})()

		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer func(err := out.Close(); if err != nil {
			logger.Error(err)
		})()

		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return nil
	})
}
