package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func bytesContainNull(b []byte) bool { return strings.IndexByte(string(b), 0) >= 0 }

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".ovx-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(content)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func createFile(workspace, relative, content string) (map[string]interface{}, error) {
	if int64(len(content)) > maxFileBytes || !utf8.ValidString(content) || strings.IndexByte(content, 0) >= 0 {
		return nil, errors.New("new file must be UTF-8 text within the file size limit")
	}
	path, err := safePath(workspace, relative)
	if err != nil {
		return nil, err
	}
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		return nil, errors.New("file already exists or is inaccessible")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	_, err = f.WriteString(content)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	// Explicit model-created files must appear in review even when a repository ignores their path.
	if out, code, err := gitHost(workspace, nil, "add", "--intent-to-add", "--force", "--", relative); err != nil || code != 0 {
		_ = os.Remove(path)
		return nil, fmt.Errorf("cannot include new file in review: %s %v", out, err)
	}
	return map[string]interface{}{"path": relative, "changed": true, "after_sha256": sha([]byte(content))}, nil
}

func deleteFile(workspace, relative, expected string) (map[string]interface{}, error) {
	if expected == "" {
		return nil, errors.New("expected_sha256 is required to delete a file")
	}
	file, err := readFile(workspace, relative)
	if err != nil {
		return nil, err
	}
	if file.SHA256 != expected {
		return nil, errors.New("stale file: expected_sha256 does not match")
	}
	path, err := safePath(workspace, relative)
	if err != nil {
		return nil, err
	}
	if err = os.Remove(path); err != nil {
		return nil, err
	}
	return map[string]interface{}{"path": relative, "changed": true, "before_sha256": file.SHA256}, nil
}
