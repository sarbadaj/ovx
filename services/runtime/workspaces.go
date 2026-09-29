package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

type workspaceMetadata struct {
	Repository     string                  `json:"repository"`
	Kind           string                  `json:"kind,omitempty"`
	SourceRevision string                  `json:"source_revision,omitempty"`
	SnapshotFiles  map[string]snapshotFile `json:"snapshot_files,omitempty"`
	BaseRevision   string                  `json:"base_revision"`
	Applied        bool                    `json:"applied"`
	Destroyed      bool                    `json:"destroyed"`
}

var taskTokenPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func validTaskToken(token string) bool { return taskTokenPattern.MatchString(token) }

type workspaceState struct {
	execution sync.Mutex
	mu        sync.Mutex
	cancelled bool
	cancel    context.CancelFunc
}

var workspaceStates sync.Map
var repositoryLocks sync.Map

func stateFor(workspace string) *workspaceState {
	_, err := os.Stat(metadataPath(workspace) + ".cancelled")
	state := &workspaceState{cancelled: err == nil}
	actual, _ := workspaceStates.LoadOrStore(workspace, state)
	return actual.(*workspaceState)
}
func (s *workspaceState) isCancelled() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.cancelled }
func repositoryLock(repository string) *sync.Mutex {
	lock, _ := repositoryLocks.LoadOrStore(repository, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func gitEnvironment() []string {
	env := []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0")
}
func gitHost(dir string, input io.Reader, args ...string) (string, int, error) {
	options := []string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "diff.external=", "-c", "status.renames=false", "-c", "core.attributesFile=/dev/null", "-c", "protocol.ext.allow=never"}
	if dir != "" {
		// Inspect configuration with a read-only command before commands that could invoke
		// clean/smudge/process or upload-pack hooks. These callbacks are host programs.
		keys, code, err := runHost(dir, nil, "git", append(options, "config", "--null", "--name-only", "--get-regexp", `^(filter\..*\.(clean|smudge|process)|uploadpack\.packobjectshook)$`)...)
		if err != nil || code > 1 {
			return keys, code, fmt.Errorf("cannot safely inspect Git configuration: %v", err)
		}
		if keys != "" {
			return "", 1, errors.New("repository config defines host Git filters or upload-pack hooks; use a checkout without those callbacks")
		}
	}
	return runHost(dir, input, "git", append(options, args...)...)
}
func resolveDirectory(input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", errors.New("repository or workspace path is required")
	}
	abs, err := filepath.Abs(expandHome(input))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("directory not found: %w", err)
	}
	st, err := os.Stat(resolved)
	if err != nil || !st.IsDir() {
		return "", errors.New("path must be a directory")
	}
	return filepath.Clean(resolved), nil
}
func validateRepository(input string) (string, error) {
	repository, err := resolveDirectory(input)
	if err != nil {
		return "", err
	}
	allowed := registeredRepository(repository)
	for _, root := range append([]string{workspaceRoot}, repositoryRoots...) {
		resolved, err := resolveDirectory(root)
		if err == nil && inside(resolved, repository) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", errors.New("connect this source folder before starting a task")
	}
	if inside(filepath.Join(workspaceRoot, ".ovx"), repository) {
		return "", errors.New("OVX internal directories cannot be registered as repositories")
	}
	return repository, nil
}
func validateTaskWorkspace(input string) (string, error) {
	workspace, err := resolveDirectory(input)
	if err != nil {
		return "", err
	}
	taskRoot := filepath.Join(workspaceRoot, ".ovx", "tasks")
	if filepath.Dir(workspace) != taskRoot {
		return "", errors.New("not an OVX task workspace")
	}
	name := filepath.Base(workspace)
	idText := name
	if parts := strings.SplitN(name, "-", 2); len(parts) == 2 {
		if !validTaskToken(parts[1]) {
			return "", errors.New("invalid task workspace token")
		}
		idText = parts[0]
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		return "", errors.New("invalid task workspace identifier")
	}
	if meta, err := readMetadata(workspace); err != nil || meta.Destroyed {
		return "", errors.New("task workspace is missing or destroyed")
	}
	if st, err := os.Lstat(filepath.Join(workspace, ".git")); err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("task workspace Git metadata is invalid")
	}
	return workspace, nil
}
func metadataPath(workspace string) string {
	return filepath.Join(workspaceRoot, ".ovx", "metadata", filepath.Base(workspace)+".json")
}
func readMetadata(workspace string) (workspaceMetadata, error) {
	var meta workspaceMetadata
	b, err := os.ReadFile(metadataPath(workspace))
	if err != nil {
		return meta, errors.New("task workspace metadata is missing")
	}
	if err = json.Unmarshal(b, &meta); err != nil {
		return meta, errors.New("task workspace metadata is invalid")
	}
	if !regexp.MustCompile(`^[0-9a-f]{40,64}$`).MatchString(meta.BaseRevision) {
		return meta, errors.New("invalid base revision in task metadata")
	}
	return meta, nil
}
func writeMetadata(workspace string, meta workspaceMetadata) error {
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(metadataPath(workspace)), 0o700); err != nil {
		return err
	}
	return atomicWrite(metadataPath(workspace), b, 0o600)
}
func cleanRepository(repository string) error {
	status, code, err := gitHost(repository, nil, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil || code != 0 {
		return fmt.Errorf("cannot inspect source repository status: %v", err)
	}
	if status != "" {
		return errors.New("source repository has uncommitted changes; save and commit or stash changes before starting or accepting a task")
	}
	return nil
}
func createTaskWorkspace(repository string, taskID int64, taskToken ...string) (string, string, error) {
	lock := repositoryLock(repository)
	lock.Lock()
	defer lock.Unlock()
	name := strconv.FormatInt(taskID, 10)
	if len(taskToken) > 0 {
		if !validTaskToken(taskToken[0]) {
			return "", "", errors.New("invalid task workspace token")
		}
		name += "-" + taskToken[0]
	}
	target := filepath.Join(workspaceRoot, ".ovx", "tasks", name)
	state := stateFor(target)
	state.execution.Lock()
	defer state.execution.Unlock()
	if meta, err := readMetadata(target); err == nil {
		if meta.Destroyed {
			return "", "", errors.New("task workspace was rejected; start a new task")
		}
		if meta.Repository != repository {
			return "", "", errors.New("task workspace belongs to another repository")
		}
		if _, err = validateTaskWorkspace(target); err != nil {
			return "", "", err
		}
		return target, meta.BaseRevision, nil
	}
	return createSnapshotWorkspace(repository, target)

}
func prepareTaskIndex(workspace, base string) error {
	meta, err := readMetadata(workspace)
	if err != nil {
		return err
	}
	if base != meta.BaseRevision {
		return errors.New("base_revision does not match task workspace")
	}
	// Git stages all changed/new/deleted files for a complete review; the model cannot modify this protected index.
	out, code, err := gitHost(workspace, nil, "add", "--all", "--", ".")
	if err != nil || code != 0 {
		return fmt.Errorf("cannot inspect task changes: %s %v", out, err)
	}
	return nil
}
func workspacePatch(workspace, base string) (string, error) {
	if err := prepareTaskIndex(workspace, base); err != nil {
		return "", err
	}
	patch, code, err := gitHost(workspace, nil, "diff", "--cached", "--binary", "--no-ext-diff", "--no-textconv", "--no-renames", base, "--")
	if err != nil || code != 0 {
		return "", fmt.Errorf("cannot generate complete task patch: %v", err)
	}
	return patch, nil
}

type ReviewFile struct {
	Path   string  `json:"path"`
	Status string  `json:"status"`
	Before *string `json:"before"`
	After  *string `json:"after"`
	Binary bool    `json:"binary,omitempty"`
}
type WorkspaceReview struct {
	Diff         string       `json:"diff"`
	Files        []ReviewFile `json:"files"`
	BaseRevision string       `json:"base_revision"`
	PatchSHA256  string       `json:"patch_sha256"`
}

func reviewWorkspace(workspace, base string) (WorkspaceReview, error) {
	review := WorkspaceReview{Files: []ReviewFile{}, BaseRevision: base}
	patch, err := workspacePatch(workspace, base)
	if err != nil {
		return review, err
	}
	review.Diff = patch
	review.PatchSHA256 = sha([]byte(patch))
	names, code, err := gitHost(workspace, nil, "diff", "--cached", "--name-status", "-z", "--no-renames", base, "--")
	if err != nil || code != 0 {
		return review, errors.New("cannot list changed files")
	}
	entries := strings.Split(strings.TrimSuffix(names, "\x00"), "\x00")
	if names == "" {
		return review, nil
	}
	total := len(patch)
	for i := 0; i+1 < len(entries); i += 2 {
		code, path := entries[i], entries[i+1]
		if _, err := safePath(workspace, path); err != nil {
			return review, fmt.Errorf("unsafe changed file %q: %w", path, err)
		}
		file := ReviewFile{Path: path, Status: "modified"}
		if code == "A" {
			file.Status = "added"
		}
		if code == "D" {
			file.Status = "deleted"
		}
		if code != "A" {
			content, c, err := gitHost(workspace, nil, "show", base+":"+path)
			if err != nil || c != 0 {
				return review, fmt.Errorf("cannot read base file %s", path)
			}
			if int64(len(content)) > maxFileBytes {
				return review, fmt.Errorf("base file %s exceeds review limit", path)
			}
			if utf8.ValidString(content) && !strings.ContainsRune(content, 0) {
				file.Before = &content
			} else {
				file.Binary = true
			}
			total += len(content)
		}
		if code != "D" {
			absolute, _ := safePath(workspace, path)
			info, err := os.Stat(absolute)
			if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileBytes {
				return review, fmt.Errorf("changed file %s is not a bounded regular file", path)
			}
			// Review the staged blob that generates the accepted patch, including Git's
			// line-ending normalization, rather than a potentially different worktree file.
			content, code, err := gitHost(workspace, nil, "show", ":"+path)
			if err != nil || code != 0 || int64(len(content)) > maxFileBytes {
				return review, fmt.Errorf("cannot read proposed file %s", path)
			}
			if utf8.ValidString(content) && !strings.ContainsRune(content, 0) {
				file.After = &content
			} else {
				file.Binary = true
			}
			total += len(content)
		}
		if total > maxOutputBytes*4 {
			return review, errors.New("review exceeds total size limit; use smaller tasks")
		}
		review.Files = append(review.Files, file)
	}
	return review, nil
}
func applyTaskWorkspace(repository, workspace, base string, expectedPatch ...string) (map[string]interface{}, error) {
	lock := repositoryLock(repository)
	lock.Lock()
	defer lock.Unlock()
	state := stateFor(workspace)
	state.execution.Lock()
	defer state.execution.Unlock()
	meta, err := readMetadata(workspace)
	if err != nil {
		return nil, err
	}
	if meta.Destroyed {
		return nil, errors.New("task workspace was destroyed")
	}
	if meta.Repository != repository || meta.BaseRevision != base {
		return nil, errors.New("repository/base revision do not match task workspace")
	}
	if meta.Applied {
		return map[string]interface{}{"applied": true, "already_applied": true, "base_revision": base}, nil
	}
	if meta.Kind != "snapshot-v1" {
		current, code, err := gitHost(repository, nil, "rev-parse", "HEAD")
		if err != nil || code != 0 || strings.TrimSpace(current) != base {
			return nil, errors.New("source repository HEAD changed; create a fresh task")
		}
	}
	review, err := reviewWorkspace(workspace, base)
	if err != nil {
		return nil, err
	}
	patch := review.Diff
	if len(expectedPatch) > 0 && expectedPatch[0] != "" && expectedPatch[0] != review.PatchSHA256 {
		return nil, errors.New("task changes differ from the reviewed patch; review the updated diff before accepting")
	}
	if patch == "" {
		return map[string]interface{}{"applied": false, "reason": "empty patch"}, nil
	}
	if meta.Kind == "snapshot-v1" {
		return applySnapshotWorkspace(repository, workspace, meta, review)
	}
	if err := cleanRepository(repository); err != nil {
		if !sourceMatchesReview(repository, review) {
			return nil, err
		}
		meta.Applied = true
		if err = writeMetadata(workspace, meta); err != nil {
			return nil, err
		}
		return map[string]interface{}{"applied": true, "already_applied": true, "base_revision": base}, nil
	}
	if out, code, err := gitHost(repository, strings.NewReader(patch), "apply", "--check", "--binary", "-"); err != nil || code != 0 {
		return nil, fmt.Errorf("patch check failed: %s", strings.TrimSpace(out))
	}
	if out, code, err := gitHost(repository, strings.NewReader(patch), "apply", "--binary", "-"); err != nil || code != 0 {
		return nil, fmt.Errorf("patch apply failed: %s", strings.TrimSpace(out))
	}
	meta.Applied = true
	if err = writeMetadata(workspace, meta); err != nil {
		return nil, err
	}
	return map[string]interface{}{"applied": true, "bytes": len(patch), "base_revision": base}, nil
}
func sourceMatchesReview(repository string, review WorkspaceReview) bool {
	status, code, err := gitHost(repository, nil, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil || code != 0 {
		return false
	}
	wanted := map[string]ReviewFile{}
	for _, file := range review.Files {
		wanted[file.Path] = file
	}
	entries := strings.Split(strings.TrimSuffix(status, "\x00"), "\x00")
	if len(entries) != len(wanted) {
		return false
	}
	for _, entry := range entries {
		if len(entry) < 4 {
			return false
		}
		file, ok := wanted[entry[3:]]
		if !ok || file.Binary {
			return false
		}
		path, err := safePath(repository, file.Path)
		if err != nil {
			return false
		}
		if file.Status == "deleted" {
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				return false
			}
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil || file.After == nil || string(b) != *file.After {
			return false
		}
	}
	return true
}
func workspaceRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return "", false
	}
	if !authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return "", false
	}
	var req DestroyWorkspaceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return "", false
	}
	workspace, err := validateTaskWorkspace(req.Workspace)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return "", false
	}
	return workspace, true
}
func handleWorkspaceDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req ApplyWorkspaceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	workspace, err := validateTaskWorkspace(req.Workspace)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	state := stateFor(workspace)
	state.execution.Lock()
	defer state.execution.Unlock()
	review, err := reviewWorkspace(workspace, req.BaseRevision)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, review)
}
func handleCancelWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, ok := workspaceRequest(w, r)
	if !ok {
		return
	}
	state := stateFor(workspace)
	state.mu.Lock()
	defer state.mu.Unlock()
	state.cancelled = true
	if state.cancel != nil {
		state.cancel()
	}
	if err := os.WriteFile(metadataPath(workspace)+".cancelled", []byte("cancelled\n"), 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
}
func handleResumeWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, ok := workspaceRequest(w, r)
	if !ok {
		return
	}
	state := stateFor(workspace)
	state.execution.Lock()
	defer state.execution.Unlock()
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := os.Remove(metadataPath(workspace) + ".cancelled"); err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	state.cancelled = false
	writeJSON(w, http.StatusOK, map[string]bool{"resumed": true})
}
