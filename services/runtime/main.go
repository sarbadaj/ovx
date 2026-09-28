package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type ExecuteRequest struct {
	Workspace string                 `json:"workspace"`
	Tool      string                 `json:"tool"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ExecuteResponse struct {
	Status   string      `json:"status"`
	Output   interface{} `json:"output,omitempty"`
	Error    string      `json:"error,omitempty"`
	ExitCode *int        `json:"exit_code,omitempty"`
}

type CreateWorkspaceRequest struct {
	Repository string `json:"repository"`
	TaskID     int64  `json:"task_id"`
}

type ApplyWorkspaceRequest struct {
	Repository          string `json:"repository"`
	Workspace           string `json:"workspace"`
	BaseRevision        string `json:"base_revision"`
	ExpectedPatchSHA256 string `json:"expected_patch_sha256,omitempty"`
}

type DestroyWorkspaceRequest struct {
	Workspace string `json:"workspace"`
}

type FileContent struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Bytes   int    `json:"bytes"`
	Content string `json:"content"`
}

var (
	workspaceRoot   string
	repositoryRoots []string
	sandboxImage    string
	runtimeToken    string
	maxFileBytes    int64
	maxOutputBytes  int
	commandTimeout  time.Duration
)

func main() {
	workspaceRoot = expandHome(getenv("OVX_WORKSPACE_ROOT", "~/ovx-workspaces"))
	sandboxImage = getenv("OVX_SANDBOX_IMAGE", "ovx-sandbox-ruby:latest")
	runtimeToken = getenv("OVX_RUNTIME_TOKEN", "")
	if runtimeToken == "" {
		log.Fatal("OVX_RUNTIME_TOKEN must be configured")
	}
	addr := getenv("OVX_RUNTIME_ADDR", "127.0.0.1:7070")
	maxFileBytes = int64(envInt("OVX_MAX_FILE_BYTES", 200000))
	maxOutputBytes = envInt("OVX_MAX_TOOL_OUTPUT_BYTES", 200000)
	commandTimeout = time.Duration(envInt("OVX_COMMAND_TIMEOUT_SECONDS", 300)) * time.Second

	rootAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(rootAbs, 0o755); err != nil {
		log.Fatal(err)
	}
	rootResolved, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		log.Fatal(err)
	}
	workspaceRoot = filepath.Clean(rootResolved)

	repositoryRoots = filepath.SplitList(os.Getenv("OVX_REPOSITORY_ROOTS"))

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/execute", handleExecute)
	mux.HandleFunc("/workspace/create", handleCreateWorkspace)
	mux.HandleFunc("/repository/register", handleRegisterRepository)
	mux.HandleFunc("/workspace/apply", handleApplyWorkspace)
	mux.HandleFunc("/workspace/destroy", handleDestroyWorkspace)
	mux.HandleFunc("/workspace/diff", handleWorkspaceDiff)
	mux.HandleFunc("/workspace/cancel", handleCancelWorkspace)
	mux.HandleFunc("/workspace/resume", handleResumeWorkspace)

	log.Printf("OVX runtime listening on http://%s", addr)
	log.Printf("workspace root: %s", workspaceRoot)
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	log.Fatal(server.ListenAndServe())
}

func handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ExecuteResponse{Status: "failed", Error: "POST required"})
		return
	}
	if !authorized(r) {
		writeJSON(w, http.StatusUnauthorized, ExecuteResponse{Status: "failed", Error: "invalid runtime token"})
		return
	}

	var req ExecuteRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ExecuteResponse{Status: "failed", Error: "invalid JSON: " + err.Error()})
		return
	}

	workspace, err := validateTaskWorkspace(req.Workspace)
	if err != nil {
		writeJSON(w, http.StatusForbidden, ExecuteResponse{Status: "failed", Error: err.Error()})
		return
	}

	state := stateFor(workspace)
	state.execution.Lock()
	defer state.execution.Unlock()
	if state.isCancelled() {
		writeJSON(w, http.StatusConflict, ExecuteResponse{Status: "failed", Error: "workspace is cancelled; retry to resume"})
		return
	}
	meta, err := readMetadata(workspace)
	if err != nil || meta.Applied || meta.Destroyed {
		writeJSON(w, http.StatusConflict, ExecuteResponse{Status: "failed", Error: "workspace is already applied or destroyed"})
		return
	}
	resp := execute(workspace, req.Tool, req.Arguments)
	status := http.StatusOK
	if resp.Status == "failed" {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, resp)
}

func authorized(r *http.Request) bool {
	return runtimeToken != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+runtimeToken)) == 1
}

func handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req CreateWorkspaceRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil || req.TaskID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	repository, err := validateRepository(req.Repository)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	workspace, revision, err := createTaskWorkspace(repository, req.TaskID)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"workspace": workspace, "base_revision": revision})
}

func handleApplyWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req ApplyWorkspaceRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	repository, err := validateRepository(req.Repository)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	workspace, err := validateTaskWorkspace(req.Workspace)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	result, err := applyTaskWorkspace(repository, workspace, req.BaseRevision, req.ExpectedPatchSHA256)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleDestroyWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req DestroyWorkspaceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	workspace := filepath.Clean(req.Workspace)
	if !filepath.IsAbs(workspace) || filepath.Dir(workspace) != filepath.Join(workspaceRoot, ".ovx", "tasks") {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "not an OVX task workspace"})
		return
	}
	meta, err := readMetadata(workspace)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	state := stateFor(workspace)
	state.mu.Lock()
	state.cancelled = true
	if state.cancel != nil {
		state.cancel()
	}
	state.mu.Unlock()
	state.execution.Lock()
	defer state.execution.Unlock()
	if err := os.RemoveAll(workspace); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	meta.Destroyed = true
	if err := writeMetadata(workspace, meta); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"destroyed": true, "workspace": workspace})
}

func runHost(dir string, stdin io.Reader, name string, args ...string) (string, int, error) {
	cmd := exec.Command(name, args...)
	if name == "git" {
		cmd.Env = gitEnvironment()
	}
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = stdin
	out, err := combinedLimited(cmd, commandTimeout)
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return out, ee.ExitCode(), nil
		}
		return out, 1, err
	}
	return out, code, nil
}

func execute(workspace, tool string, args map[string]interface{}) ExecuteResponse {
	switch tool {
	case "list_files":
		out, err := listFiles(workspace)
		return result(out, err)
	case "read_file":
		path, _ := args["path"].(string)
		out, err := readFile(workspace, path)
		return result(out, err)
	case "search_code":
		query, _ := args["query"].(string)
		out, err := searchCode(workspace, query)
		return result(out, err)
	case "create_file":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		out, err := createFile(workspace, path, content)
		return result(out, err)
	case "delete_file":
		path, _ := args["path"].(string)
		expected, _ := args["expected_sha256"].(string)
		out, err := deleteFile(workspace, path, expected)
		return result(out, err)
	case "apply_patch":
		path, _ := args["path"].(string)
		oldText, _ := args["old"].(string)
		newText, _ := args["new"].(string)
		expected, _ := args["expected_sha256"].(string)
		out, err := applyPatch(workspace, path, oldText, newText, expected)
		return result(out, err)
	case "run_tests":
		command, _ := args["command"].(string)
		if err := validateTestCommand(command); err != nil {
			return result(nil, err)
		}
		out, code, err := runSandboxedShell(workspace, command)
		resp := result(out, err)
		resp.ExitCode = &code
		return resp
	case "run_command":
		command, _ := args["command"].(string)
		if err := validateApprovedCommand(command); err != nil {
			return result(nil, err)
		}
		out, code, err := runSandboxedShell(workspace, command)
		resp := result(out, err)
		resp.ExitCode = &code
		return resp
	case "git_status":
		out, code, err := gitHost(workspace, nil, "status", "--short")
		resp := result(out, err)
		resp.ExitCode = &code
		return resp
	case "git_diff":
		meta, err := readMetadata(workspace)
		if err != nil {
			return result(nil, err)
		}
		out, err := workspacePatch(workspace, meta.BaseRevision)
		return result(out, err)
	case "git_log":
		out, code, err := gitHost(workspace, nil, "log", "-n", "10", "--oneline", "--decorate")
		resp := result(out, err)
		resp.ExitCode = &code
		return resp
	default:
		return ExecuteResponse{Status: "failed", Error: "unsupported tool"}
	}
}

func inside(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func safePath(workspace, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) || strings.ContainsRune(relative, 0) {
		return "", errors.New("a repository-relative path is required")
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("path escapes workspace")
	}
	candidate := workspace
	for _, part := range strings.Split(clean, string(os.PathSeparator)) {
		if strings.EqualFold(part, ".git") || strings.EqualFold(part, ".ovx") {
			return "", errors.New("repository metadata paths are protected")
		}
		candidate = filepath.Join(candidate, part)
		info, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("symlink paths are not allowed")
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return "", errors.New("special files are not allowed")
		}
	}
	return candidate, nil
}

func listFiles(workspace string) ([]string, error) {
	var files []string
	skip := map[string]bool{
		".git": true, ".ovx": true, "node_modules": true, "vendor": true,
		"tmp": true, "log": true, "coverage": true,
	}
	stop := errors.New("file limit reached")
	err := filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == workspace {
			return nil
		}
		if info.IsDir() && skip[info.Name()] {
			return filepath.SkipDir
		}
		if info.Mode().IsRegular() {
			rel, _ := filepath.Rel(workspace, path)
			files = append(files, filepath.ToSlash(rel))
			if len(files) >= 2000 {
				return stop
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, stop) {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func readFile(workspace, relative string) (FileContent, error) {
	path, err := safePath(workspace, relative)
	if err != nil {
		return FileContent{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return FileContent{}, err
	}
	if !info.Mode().IsRegular() {
		return FileContent{}, errors.New("only regular files can be read")
	}
	f, err := os.Open(path)
	if err != nil {
		return FileContent{}, err
	}
	defer f.Close()

	b, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return FileContent{}, err
	}
	if int64(len(b)) > maxFileBytes {
		return FileContent{}, fmt.Errorf("file exceeds %d bytes", maxFileBytes)
	}
	if !utf8.Valid(b) || bytesContainNull(b) {
		return FileContent{}, errors.New("binary files are not supported by text tools")
	}
	return FileContent{Path: relative, SHA256: sha(b), Bytes: len(b), Content: string(b)}, nil
}

func searchCode(workspace, query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", errors.New("query is required")
	}
	cmd := exec.Command("rg", "-n", "--hidden", "--glob", "!.ovx/**", "--glob", "!.git/**", "--glob", "!node_modules/**", "--glob", "!vendor/**", "--", query, ".")
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "RIPGREP_CONFIG_PATH=")
	out, err := combinedLimited(cmd, commandTimeout)
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return out, nil
	}
	return out, err
}

func applyPatch(workspace, relative, oldText, newText, expectedSHA string) (map[string]interface{}, error) {
	if oldText == "" {
		return nil, errors.New("old text must not be empty")
	}
	path, err := safePath(workspace, relative)
	if err != nil {
		return nil, err
	}
	file, err := readFile(workspace, relative)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	b := []byte(file.Content)
	beforeSHA := sha(b)
	if expectedSHA != "" && expectedSHA != beforeSHA {
		return nil, fmt.Errorf("stale file: expected sha256 %s but current is %s", expectedSHA, beforeSHA)
	}
	content := string(b)
	count := strings.Count(content, oldText)
	if count != 1 {
		return nil, fmt.Errorf("old text must match exactly once; matched %d times", count)
	}
	updated := []byte(strings.Replace(content, oldText, newText, 1))

	if !utf8.Valid(updated) || bytesContainNull(updated) {
		return nil, errors.New("patch must contain UTF-8 text")
	}
	if int64(len(updated)) > maxFileBytes {
		return nil, errors.New("updated file exceeds size limit")
	}
	if err := atomicWrite(path, updated, info.Mode().Perm()); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"path": relative, "changed": true,
		"before_sha256": beforeSHA, "after_sha256": sha(updated),
	}, nil
}

var safeArg = `[A-Za-z0-9_./:#=,@+%-]+`
var testPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^ruby ` + safeArg + `_test\.rb$`),
	regexp.MustCompile(`^bundle exec rspec(?: ` + safeArg + `)*$`),
	regexp.MustCompile(`^(?:bin/rails|bundle exec rails) test(?: ` + safeArg + `)*$`),
	regexp.MustCompile(`^(?:rake|bundle exec rake) test(?: ` + safeArg + `)*$`),
}
var approvedCommandPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^bundle exec rubocop(?: ` + safeArg + `)*$`),
	regexp.MustCompile(`^(?:bin/rails|bundle exec rails) routes(?: ` + safeArg + `)*$`),
	regexp.MustCompile(`^ruby -c ` + safeArg + `$`),
}

func validateTestCommand(command string) error {
	c := strings.TrimSpace(command)
	if c == "" {
		return errors.New("command is required")
	}
	if hasForbiddenShellSyntax(c) {
		return errors.New("shell metacharacters are not allowed")
	}
	for _, re := range testPatterns {
		if re.MatchString(c) {
			return nil
		}
	}
	return errors.New("command is not in the test-command allowlist")
}

func validateApprovedCommand(command string) error {
	c := strings.TrimSpace(command)
	if c == "" {
		return errors.New("command is required")
	}
	if hasForbiddenShellSyntax(c) {
		return errors.New("shell metacharacters are not allowed")
	}
	for _, re := range approvedCommandPatterns {
		if re.MatchString(c) {
			return nil
		}
	}
	return errors.New("approved command is still outside the MVP runtime allowlist")
}

func hasForbiddenShellSyntax(command string) bool {
	return strings.ContainsAny(command, ";|&`$><\\\n\r")
}

func runSandboxedShell(workspace, command string) (string, int, error) {
	return runDocker(workspace, strings.Fields(command))
}

func runSandboxedArgs(workspace string, command []string) (string, int, error) {
	return runDocker(workspace, command)
}

func runDocker(workspace string, command []string) (string, int, error) {
	state := stateFor(workspace)
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	state.mu.Lock()
	if state.cancelled {
		state.mu.Unlock()
		return "", 1, errors.New("workspace is cancelled")
	}
	state.cancel = cancel
	state.mu.Unlock()
	defer func() { state.mu.Lock(); state.cancel = nil; state.mu.Unlock() }()
	name := fmt.Sprintf("ovx-%s-%d", filepath.Base(workspace), time.Now().UnixNano())
	args := []string{
		"run", "--rm", "--pull=never", "--name", name, "--network", "none",
		"--cpus", "2", "--memory", "2g", "--pids-limit", "256",
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=256m",
		"-e", "HOME=/tmp", "-e", "RAILS_ENV=test", "-e", "RACK_ENV=test",
		"-e", "GIT_CONFIG_NOSYSTEM=1", "-e", "GIT_CONFIG_GLOBAL=/dev/null",
		"--mount", "type=bind,src=" + workspace + ",dst=/workspace",
		"--mount", "type=bind,src=" + filepath.Join(workspace, ".git") + ",dst=/workspace/.git,readonly",
		"-w", "/workspace", sandboxImage,
	}
	args = append(args, command...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := combinedLimited(cmd, commandTimeout)
	if ctx.Err() != nil || (err != nil && !isExitError(err)) {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = exec.CommandContext(cleanup, "docker", "rm", "-f", name).Run()
		if ctx.Err() != nil {
			return out, 1, fmt.Errorf("sandbox stopped: %w", ctx.Err())
		}
		return out, 1, err
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return out, ee.ExitCode(), nil
		}
		return out, 1, err
	}
	return out, 0, nil
}

func isExitError(err error) bool { var exit *exec.ExitError; return errors.As(err, &exit) }

type limitedBuffer struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := b.limit - len(b.data)
	if remaining < n {
		b.truncated = true
		p = p[:remaining]
	}
	b.data = append(b.data, p...)
	return n, nil
}
func combinedLimited(cmd *exec.Cmd, timeout time.Duration) (string, error) {
	buf := &limitedBuffer{limit: maxOutputBytes}
	cmd.Stdout, cmd.Stderr = buf, buf
	// Ensure descendants that inherit pipes cannot keep a completed command alive.
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var err error
	select {
	case err = <-done:
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-done
		err = errors.New("command timed out")
	}
	buf.mu.Lock()
	defer buf.mu.Unlock()
	if buf.truncated {
		return string(buf.data), errors.New("command output exceeded limit; result is incomplete")
	}
	return string(buf.data), err
}

func result(output interface{}, err error) ExecuteResponse {
	if err != nil {
		return ExecuteResponse{Status: "failed", Output: output, Error: err.Error()}
	}
	return ExecuteResponse{Status: "completed", Output: output}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := getenv(key, strconv.Itoa(fallback))
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		if path == "~" {
			return home
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}
