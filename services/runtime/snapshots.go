package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type snapshotFile struct {
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
}

type registeredFolder struct {
	Path string `json:"path"`
}

func registrationPath(repository string) string {
	return filepath.Join(workspaceRoot, ".ovx", "registrations", sha([]byte(repository))+".json")
}
func registeredRepository(repository string) bool {
	b, err := os.ReadFile(registrationPath(repository))
	if err != nil {
		return false
	}
	var registered registeredFolder
	return json.Unmarshal(b, &registered) == nil && registered.Path == repository
}
func handleRegisterRepository(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	if !authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req registeredFolder
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	repository, err := resolveDirectory(req.Path)
	if err == nil && inside(filepath.Join(workspaceRoot, ".ovx"), repository) {
		err = errors.New("OVX internal directories cannot be registered as source folders")
	}
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	file := registrationPath(repository)
	if err = os.MkdirAll(filepath.Dir(file), 0o700); err == nil {
		data, _ := json.Marshal(registeredFolder{Path: repository})
		err = atomicWrite(file, data, 0o600)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"path": repository, "registered": true})
}

// Read-only Git discovery never runs attributes, filters, hooks, or updates the source index.
// It also works from a subfolder of an existing repository.
func sourceGit(repository string, args ...string) (string, int, error) {
	options := []string{"--no-optional-locks", "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "protocol.ext.allow=never"}
	return runHost(repository, nil, "git", append(options, args...)...)
}

func snapshotExcluded(relative string) bool {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for _, part := range parts {
		switch strings.ToLower(part) {
		case ".git", ".ovx", "node_modules", ".bundle", ".venv", "venv", "__pycache__", ".cache", "coverage", "tmp", "log", ".ds_store":
			return true
		}
	}
	lower := strings.ToLower(filepath.Base(relative))
	if lower == ".env" || (strings.HasPrefix(lower, ".env.") && lower != ".env.example" && lower != ".env.sample" && lower != ".env.template") {
		return true
	}
	if lower == "master.key" || lower == "id_rsa" || lower == "id_ed25519" || strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".p12") {
		return true
	}
	return strings.HasPrefix(filepath.ToSlash(relative), "vendor/bundle/")
}

func createSnapshotWorkspace(repository, target string) (workspace string, revision string, err error) {
	if err = os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", "", err
	}
	if _, err = os.Lstat(target); !os.IsNotExist(err) {
		return "", "", errors.New("task workspace already exists without valid metadata; inspect it before removing")
	}
	if err = os.Mkdir(target, 0o700); err != nil {
		return "", "", err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(target)
		}
	}()
	if out, code, e := gitHost(target, nil, "init", "--quiet"); e != nil || code != 0 {
		return "", "", fmt.Errorf("cannot initialize task snapshot: %s %v", out, e)
	}
	// Preserve exact source bytes. Repository attributes must not rewrite content during snapshots or reviews.
	if err = os.WriteFile(filepath.Join(target, ".git", "info", "attributes"), []byte("* -filter -text -ident -working-tree-encoding\n"), 0o600); err != nil {
		return "", "", err
	}
	excludes := ".ovx/\nnode_modules/\n.bundle/\n.venv/\nvenv/\n__pycache__/\n.cache/\ncoverage/\ntmp/\nlog/\nvendor/bundle/\n"
	if err = os.WriteFile(filepath.Join(target, ".git", "info", "exclude"), []byte(excludes), 0o600); err != nil {
		return "", "", err
	}
	// Git's traversal applies the selected folder's nested .gitignore files without initializing or changing it.
	names, code, e := gitHost(target, nil, "--git-dir="+filepath.Join(target, ".git"), "--work-tree="+repository, "ls-files", "--others", "--exclude-standard", "-z", "--")
	if e != nil || code != 0 {
		return "", "", fmt.Errorf("cannot enumerate source folder: %s %v", names, e)
	}
	paths := map[string]bool{}
	for _, path := range strings.Split(names, "\x00") {
		if path != "" {
			paths[path] = true
		}
	}
	// Existing tracked files remain part of the snapshot even when their names match a current ignore rule.
	tracked, code, e := sourceGit(repository, "ls-files", "--cached", "-z", "--")
	if e == nil && code == 0 {
		for _, path := range strings.Split(tracked, "\x00") {
			if path != "" {
				paths[path] = true
			}
		}
	}
	sourceRevision := ""
	if out, code, e := sourceGit(repository, "rev-parse", "--verify", "HEAD"); e == nil && code == 0 {
		sourceRevision = strings.TrimSpace(out)
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	metadata := workspaceMetadata{Repository: repository, Kind: "snapshot-v1", SourceRevision: sourceRevision, SnapshotFiles: map[string]snapshotFile{}}
	var total int64
	limit := int64(envInt("OVX_MAX_SNAPSHOT_BYTES", 512*1024*1024))
	countLimit := envInt("OVX_MAX_SNAPSHOT_FILES", 20000)
	for _, relative := range ordered {
		if snapshotExcluded(relative) {
			continue
		}
		source, e := safePath(repository, relative)
		if e != nil {
			continue
		} // Symlinks, metadata, and special files never enter a task mount.
		if inside(filepath.Join(workspaceRoot, ".ovx"), source) {
			continue
		}
		info, e := os.Lstat(source)
		if os.IsNotExist(e) {
			continue
		} // Locally deleted tracked files stay deleted in the baseline.
		if e != nil {
			return "", "", e
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if len(metadata.SnapshotFiles) >= countLimit || total+info.Size() > limit {
			return "", "", errors.New("source snapshot exceeds configured file/byte limits; ignore generated files or connect a smaller folder")
		}
		destination := filepath.Join(target, relative)
		if err = os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return "", "", err
		}
		record, e := copySnapshotFile(source, destination, info, limit-total)
		if e != nil {
			return "", "", fmt.Errorf("snapshot %s: %w", relative, e)
		}
		metadata.SnapshotFiles[relative] = record
		total += info.Size()
	}
	// Force-add only copied source files; ignored generated outputs created later remain excluded.
	if out, code, e := gitHost(target, nil, "add", "--all", "--force", "--", "."); e != nil || code != 0 {
		return "", "", fmt.Errorf("cannot stage private snapshot: %s %v", out, e)
	}
	if out, code, e := gitHost(target, nil, "-c", "user.name=OVX Snapshot", "-c", "user.email=local@ovx.invalid", "commit", "--quiet", "--allow-empty", "-m", "OVX private source snapshot"); e != nil || code != 0 {
		return "", "", fmt.Errorf("cannot record private snapshot: %s %v", out, e)
	}
	revision, code, err = gitHost(target, nil, "rev-parse", "HEAD")
	if err != nil || code != 0 {
		return "", "", errors.New("cannot resolve private snapshot revision")
	}
	revision = strings.TrimSpace(revision)
	metadata.BaseRevision = revision
	if err = writeMetadata(target, metadata); err != nil {
		return "", "", err
	}
	return target, revision, nil
}

func copySnapshotFile(source, destination string, info os.FileInfo, remaining int64) (snapshotFile, error) {
	in, err := os.Open(source)
	if err != nil {
		return snapshotFile{}, err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return snapshotFile{}, err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(in, remaining+1))
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return snapshotFile{}, err
	}
	after, err := in.Stat()
	if err != nil {
		return snapshotFile{}, err
	}
	if n != info.Size() || n > remaining || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return snapshotFile{}, errors.New("file changed while snapshotting; retry the task")
	}
	if err = os.Chmod(destination, info.Mode().Perm()); err != nil {
		return snapshotFile{}, err
	}
	return snapshotFile{SHA256: hex.EncodeToString(hash.Sum(nil)), Mode: uint32(info.Mode().Perm())}, nil
}

type snapshotChange struct {
	Path    string
	Before  *snapshotFile
	After   *snapshotFile
	Content []byte
}

func matchesSnapshotFile(path string, wanted *snapshotFile) bool {
	info, err := os.Lstat(path)
	if wanted == nil {
		return os.IsNotExist(err)
	}
	if err != nil || !info.Mode().IsRegular() || uint32(info.Mode().Perm()) != wanted.Mode {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == wanted.SHA256
}
func proposedFile(workspace, path string, before *snapshotFile) ([]byte, *snapshotFile, error) {
	content, code, err := gitHost(workspace, nil, "show", ":"+path)
	if err != nil || code != 0 {
		return nil, nil, fmt.Errorf("cannot read proposed file %s", path)
	}
	entry, code, err := gitHost(workspace, nil, "ls-files", "--stage", "-z", "--", path)
	if err != nil || code != 0 || len(entry) < 6 {
		return nil, nil, fmt.Errorf("cannot read proposed file mode %s", path)
	}
	mode, err := strconv.ParseUint(entry[:6], 8, 32)
	if err != nil || (mode != 0o100644 && mode != 0o100755) {
		return nil, nil, fmt.Errorf("unsupported proposed file type %s", path)
	}
	perm := uint32(0o644)
	if before != nil {
		perm = before.Mode
	}
	if mode == 0o100755 {
		if perm&0o111 == 0 {
			perm |= 0o111
		}
	} else {
		perm &^= 0o111
	}
	data := []byte(content)
	return data, &snapshotFile{SHA256: sha(data), Mode: perm}, nil
}
func applySnapshotWorkspace(repository, workspace string, meta workspaceMetadata, review WorkspaceReview) (map[string]interface{}, error) {
	changes := []snapshotChange{}
	allApplied := true
	for _, file := range review.Files {
		path, err := safePath(repository, file.Path)
		if err != nil {
			return nil, fmt.Errorf("source conflict at %s: %w", file.Path, err)
		}
		change := snapshotChange{Path: path}
		if before, exists := meta.SnapshotFiles[file.Path]; exists {
			copy := before
			change.Before = &copy
		}
		if file.Status != "deleted" {
			change.Content, change.After, err = proposedFile(workspace, file.Path, change.Before)
			if err != nil {
				return nil, err
			}
		}
		after := matchesSnapshotFile(path, change.After)
		if !after && !matchesSnapshotFile(path, change.Before) {
			return nil, fmt.Errorf("source conflict: %s changed since the task snapshot; review your edits and retry", file.Path)
		}
		allApplied = allApplied && after
		changes = append(changes, change)
	}
	// All touched files are checked before changing any source content. The user's index is never read or written here.
	for _, change := range changes {
		if matchesSnapshotFile(change.Path, change.After) {
			continue
		}
		if !matchesSnapshotFile(change.Path, change.Before) {
			return nil, fmt.Errorf("source conflict: %s changed during acceptance", change.Path)
		}
		if change.After == nil {
			if err := os.Remove(change.Path); err != nil {
				return nil, err
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(change.Path), 0o755); err != nil {
				return nil, err
			}
			if err := atomicWrite(change.Path, change.Content, os.FileMode(change.After.Mode)); err != nil {
				return nil, err
			}
		}
	}
	meta.Applied = true
	if err := writeMetadata(workspace, meta); err != nil {
		return nil, err
	}
	return map[string]interface{}{"applied": true, "already_applied": allApplied, "bytes": len(review.Diff), "base_revision": meta.BaseRevision}, nil
}
