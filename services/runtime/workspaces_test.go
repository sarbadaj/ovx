package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func runtimeFixture(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspaceRoot = root
	repositoryRoots = nil
	workspaceStates = sync.Map{}
	repositoryLocks = sync.Map{}
	runtimeToken = "test-token"
	maxFileBytes = 200000
	maxOutputBytes = 200000
	commandTimeout = 10 * time.Second
	repository := filepath.Join(root, "source")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repository, "init", "--quiet")
	writeTestFile(t, filepath.Join(repository, "calculator.rb"), "def add(a, b)\n  a - b\nend\n")
	writeTestFile(t, filepath.Join(repository, "obsolete.txt"), "old content\n")
	mustGit(t, repository, "add", ".")
	mustGit(t, repository, "-c", "user.name=OVX Test", "-c", "user.email=ovx@example.invalid", "commit", "--quiet", "-m", "initial")
	return repository
}
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, code, err := gitHost(dir, nil, args...)
	if err != nil || code != 0 {
		t.Fatalf("git %v: %s (%v)", args, out, err)
	}
	return strings.TrimSpace(out)
}
func createFixtureWorkspace(t *testing.T, repository string) (string, string) {
	t.Helper()
	workspace, base, err := createTaskWorkspace(repository, 1)
	if err != nil {
		t.Fatal(err)
	}
	return workspace, base
}
func postRuntime(t *testing.T, handler http.HandlerFunc, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+runtimeToken)
	res := httptest.NewRecorder()
	handler(res, req)
	return res
}

func TestWorkspaceReviewAndAcceptIncludesNewDeletedFiles(t *testing.T) {
	repository := runtimeFixture(t)
	workspace, base := createFixtureWorkspace(t, repository)
	if again, revision, err := createTaskWorkspace(repository, 1); err != nil || again != workspace || revision != base {
		t.Fatalf("create not idempotent: %s %s %v", again, revision, err)
	}
	if _, err := applyPatch(workspace, "calculator.rb", "a - b", "a + b", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := createFile(workspace, "lib/new file.rb", "# new\n"); err != nil {
		t.Fatal(err)
	}
	original, err := readFile(workspace, "obsolete.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = deleteFile(workspace, "obsolete.txt", original.SHA256); err != nil {
		t.Fatal(err)
	}
	review, err := reviewWorkspace(workspace, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Files) != 3 || !strings.Contains(review.Diff, "new file.rb") || !strings.Contains(review.Diff, "deleted file mode") {
		t.Fatalf("incomplete review: %+v", review)
	}
	byPath := map[string]ReviewFile{}
	for _, file := range review.Files {
		byPath[file.Path] = file
	}
	modified, added, deleted := byPath["calculator.rb"], byPath["lib/new file.rb"], byPath["obsolete.txt"]
	if modified.Before == nil || modified.After == nil || !strings.Contains(*modified.Before, "a - b") || !strings.Contains(*modified.After, "a + b") {
		t.Fatal("review did not preserve immutable before/after")
	}
	if added.Status != "added" || added.Before != nil || added.After == nil {
		t.Fatalf("bad new file review: %+v", added)
	}
	if deleted.Status != "deleted" || deleted.After != nil || deleted.Before == nil {
		t.Fatalf("bad deleted file review: %+v", deleted)
	}
	if err := cleanRepository(repository); err != nil {
		t.Fatalf("agent changed source before acceptance: %v", err)
	}
	result, err := applyTaskWorkspace(repository, workspace, base)
	if err != nil || result["applied"] != true {
		t.Fatalf("accept failed: %+v %v", result, err)
	}
	content, _ := os.ReadFile(filepath.Join(repository, "calculator.rb"))
	if !strings.Contains(string(content), "a + b") {
		t.Fatal("accepted file not changed")
	}
	if _, err := os.Stat(filepath.Join(repository, "lib/new file.rb")); err != nil {
		t.Fatal("new file was not accepted")
	}
	if _, err := os.Stat(filepath.Join(repository, "obsolete.txt")); !os.IsNotExist(err) {
		t.Fatal("deleted file was not deleted")
	}
	result, err = applyTaskWorkspace(repository, workspace, base)
	if err != nil || result["already_applied"] != true {
		t.Fatalf("accept not idempotent: %+v %v", result, err)
	}
	res := postRuntime(t, handleExecute, ExecuteRequest{Workspace: workspace, Tool: "create_file", Arguments: map[string]interface{}{"path": "late.txt", "content": "late"}})
	if res.Code != http.StatusConflict {
		t.Fatal("applied workspace still executable")
	}
}

func TestAcceptRefusesChangedSourcesAndWrongBase(t *testing.T) {
	repository := runtimeFixture(t)
	workspace, base := createFixtureWorkspace(t, repository)
	_, err := applyPatch(workspace, "calculator.rb", "a - b", "a + b", "")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(repository, "user-change.txt"), "preserve this")
	if _, err := applyTaskWorkspace(repository, workspace, base); err != nil {
		t.Fatalf("accepted workspace should preserve unrelated user work: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, "user-change.txt")); err != nil {
		t.Fatal("unrelated user work was not preserved")
	}
	if _, err := applyTaskWorkspace(repository, workspace, strings.Repeat("a", 40)); err == nil {
		t.Fatal("accepted wrong revision")
	}
	other := filepath.Join(workspaceRoot, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := applyTaskWorkspace(other, workspace, base); err == nil {
		t.Fatal("accepted into wrong repository")
	}
	// The private snapshot remains applicable even if the source repository HEAD moves.
}

func TestAcceptRecoversLostResponseWithoutOverwritingOtherWork(t *testing.T) {
	repository := runtimeFixture(t)
	workspace, base := createFixtureWorkspace(t, repository)
	_, err := applyPatch(workspace, "calculator.rb", "a - b", "a + b", "")
	if err != nil {
		t.Fatal(err)
	}
	patch, err := workspacePatch(workspace, base)
	if err != nil {
		t.Fatal(err)
	}
	if out, code, err := gitHost(repository, strings.NewReader(patch), "apply", "-"); err != nil || code != 0 {
		t.Fatalf("simulate apply: %s %v", out, err)
	}
	writeTestFile(t, filepath.Join(repository, "unrelated.txt"), "user edit")
	result, err := applyTaskWorkspace(repository, workspace, base)
	if err != nil || result["already_applied"] != true {
		t.Fatalf("lost response did not recover: %+v %v", result, err)
	}
}

func TestCreateAllowsDirtyRepository(t *testing.T) {
	repository := runtimeFixture(t)
	writeTestFile(t, filepath.Join(repository, "calculator.rb"), "my edits")
	if _, _, err := createTaskWorkspace(repository, 1); err != nil {
		t.Fatalf("dirty worktree should be snapshotted: %v", err)
	}
}

func TestSourceValidationAndTaskOnlyExecution(t *testing.T) {
	repository := runtimeFixture(t)
	if _, err := validateRepository(repository); err != nil {
		t.Fatal(err)
	}
	if _, err := validateTaskWorkspace(repository); err == nil {
		t.Fatal("source accepted as a task")
	}
	res := postRuntime(t, handleExecute, ExecuteRequest{Workspace: repository, Tool: "read_file", Arguments: map[string]interface{}{"path": "calculator.rb"}})
	if res.Code != http.StatusForbidden {
		t.Fatalf("source directly executable: %d", res.Code)
	}
	external, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mustGit(t, external, "init", "--quiet")
	if _, err := validateRepository(external); err == nil {
		t.Fatal("unregistered external root accepted")
	}
	repositoryRoots = []string{external}
	if _, err := validateRepository(external); err != nil {
		t.Fatal(err)
	}
}

func TestFileSafetyAndStaleEdits(t *testing.T) {
	repository := runtimeFixture(t)
	workspace, _ := createFixtureWorkspace(t, repository)
	for _, path := range []string{"../calculator.rb", "/tmp/file", ".git/config", "a/../.git/config", ".Git/config", ".ovx/task.json"} {
		if _, err := safePath(workspace, path); err == nil {
			t.Errorf("unsafe path accepted: %q", path)
		}
	}
	if err := os.Symlink(repository, filepath.Join(workspace, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := readFile(workspace, "link/calculator.rb"); err == nil {
		t.Fatal("followed directory symlink")
	}
	if _, err := createFile(workspace, "link/new.txt", "bad"); err == nil {
		t.Fatal("wrote through symlink")
	}
	if _, err := applyPatch(workspace, "calculator.rb", "a - b", "a + b", "stale"); err == nil {
		t.Fatal("stale patch accepted")
	}
	if _, err := deleteFile(workspace, "calculator.rb", ""); err == nil {
		t.Fatal("delete without hash accepted")
	}
	if _, err := createFile(workspace, "calculator.rb", "clobber"); err == nil {
		t.Fatal("existing file overwritten")
	}
	if _, err := applyPatch(workspace, "calculator.rb", "a - b", "a\x00b", ""); err == nil {
		t.Fatal("binary patch accepted")
	}
}

func TestCancellationPersistsAndResumeAllowsTools(t *testing.T) {
	repository := runtimeFixture(t)
	workspace, _ := createFixtureWorkspace(t, repository)
	state := stateFor(workspace)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state.cancel = cancel
	res := postRuntime(t, handleCancelWorkspace, DestroyWorkspaceRequest{Workspace: workspace})
	if res.Code != http.StatusOK || ctx.Err() != context.Canceled {
		t.Fatalf("cancel did not stop active command: %d %v", res.Code, ctx.Err())
	}
	res = postRuntime(t, handleExecute, ExecuteRequest{Workspace: workspace, Tool: "list_files"})
	if res.Code != http.StatusConflict {
		t.Fatal("cancelled workspace allowed another tool")
	}
	workspaceStates.Delete(workspace)
	if !stateFor(workspace).isCancelled() {
		t.Fatal("cancellation lost after runtime restart")
	}
	res = postRuntime(t, handleResumeWorkspace, DestroyWorkspaceRequest{Workspace: workspace})
	if res.Code != http.StatusOK {
		t.Fatal(res.Body.String())
	}
	res = postRuntime(t, handleExecute, ExecuteRequest{Workspace: workspace, Tool: "list_files"})
	if res.Code != http.StatusOK {
		t.Fatal(res.Body.String())
	}
}

func TestRejectIsIdempotentAndDoesNotTouchSource(t *testing.T) {
	repository := runtimeFixture(t)
	workspace, _ := createFixtureWorkspace(t, repository)
	for i := 0; i < 2; i++ {
		res := postRuntime(t, handleDestroyWorkspace, DestroyWorkspaceRequest{Workspace: workspace})
		if res.Code != http.StatusOK {
			t.Fatal(res.Body.String())
		}
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatal("task workspace remained after rejection")
	}
	if err := cleanRepository(repository); err != nil {
		t.Fatal(err)
	}
	if _, _, err := createTaskWorkspace(repository, 1); err == nil {
		t.Fatal("rejected workspace was recreated")
	}
}

func TestAuthRequiresTokenAndOutputLimitIsNotSilent(t *testing.T) {
	runtimeFixture(t)
	req := httptest.NewRequest(http.MethodPost, "/execute", strings.NewReader(`{}`))
	res := httptest.NewRecorder()
	handleExecute(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatal("unauthenticated execution accepted")
	}
	runtimeToken = ""
	if authorized(req) {
		t.Fatal("empty server token disables authentication")
	}
	maxOutputBytes = 4
	out, err := combinedLimited(exec.Command("printf", "123456789"), time.Second)
	if out != "1234" || err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("silent/truncated output: %q %v", out, err)
	}
	started := time.Now()
	_, err = combinedLimited(exec.Command("sleep", "5"), 30*time.Millisecond)
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatalf("timeout failed: %v", err)
	}
}

func TestDockerExecutionAndGitProtection(t *testing.T) {
	if os.Getenv("OVX_TEST_DOCKER") != "1" {
		t.Skip("set OVX_TEST_DOCKER=1 for real sandbox integration")
	}
	repository := runtimeFixture(t)
	sandboxImage = getenv("OVX_SANDBOX_IMAGE", "ovx-sandbox-ruby:latest")
	workspace, _ := createFixtureWorkspace(t, repository)
	commandTimeout = 30 * time.Second
	out, code, err := runDocker(workspace, []string{"ruby", "-e", `raise unless ENV['RAILS_ENV'] == 'test'; begin; File.write('.git/config', 'corrupt'); abort 'metadata writable'; rescue Errno::EROFS, Errno::EACCES; puts 'metadata protected'; end`})
	if err != nil || code != 0 || !strings.Contains(out, "metadata protected") {
		t.Fatalf("sandbox protection failed: %s %d %v", out, code, err)
	}
	commandTimeout = 500 * time.Millisecond
	started := time.Now()
	_, _, err = runDocker(workspace, []string{"ruby", "-e", "sleep 30"})
	if err == nil || time.Since(started) > 15*time.Second {
		t.Fatalf("sandbox timeout failed: %v", err)
	}
}

func TestGitHostDoesNotInvokeRepositoryCallbacks(t *testing.T) {
	repository := runtimeFixture(t)
	marker := filepath.Join(workspaceRoot, "host-command-was-run")
	for _, key := range []string{"core.fsmonitor", "core.hooksPath"} {
		cmd := exec.Command("git", "config", key, marker)
		cmd.Dir = repository
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("set callback: %s %v", out, err)
		}
	}
	if err := cleanRepository(repository); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"filter.unsafe.clean", "filter.unsafe.smudge", "filter.unsafe.process", "uploadpack.packObjectsHook"} {
		cmd := exec.Command("git", "config", key, "touch "+marker)
		cmd.Dir = repository
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("set callback: %s %v", out, err)
		}
		if _, _, err := createTaskWorkspace(repository, 1); err != nil {
			t.Fatalf("snapshot should not invoke host callback (%s): %v", key, err)
		}
		cmd = exec.Command("git", "config", "--unset", key)
		cmd.Dir = repository
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("unset callback: %s %v", out, err)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("repository executed a host callback")
	}
}

func TestReviewHashRejectsChangedArtifact(t *testing.T) {
	repository := runtimeFixture(t)
	workspace, base := createFixtureWorkspace(t, repository)
	if _, err := applyPatch(workspace, "calculator.rb", "a - b", "a + b", ""); err != nil {
		t.Fatal(err)
	}
	review, err := reviewWorkspace(workspace, base)
	if err != nil {
		t.Fatal(err)
	}
	if review.PatchSHA256 != sha([]byte(review.Diff)) {
		t.Fatal("review hash does not match patch")
	}
	if _, err := createFile(workspace, "another.rb", "# added later\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := applyTaskWorkspace(repository, workspace, base, review.PatchSHA256); err == nil || !strings.Contains(err.Error(), "reviewed patch") {
		t.Fatalf("changed artifact accepted: %v", err)
	}
	if err := cleanRepository(repository); err != nil {
		t.Fatal("stale accept changed source")
	}
}

func TestIgnoredNewFilesRemainReviewable(t *testing.T) {
	repository := runtimeFixture(t)
	writeTestFile(t, filepath.Join(repository, ".gitignore"), "generated/\n")
	mustGit(t, repository, "add", ".gitignore")
	mustGit(t, repository, "-c", "user.name=OVX Test", "-c", "user.email=ovx@example.invalid", "commit", "--quiet", "-m", "ignore generated files")
	workspace, base := createFixtureWorkspace(t, repository)
	if _, err := createFile(workspace, "generated/result.rb", "# explicit new file\n"); err != nil {
		t.Fatal(err)
	}
	review, err := reviewWorkspace(workspace, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Files) != 1 || review.Files[0].Path != "generated/result.rb" {
		t.Fatal("ignored explicit file disappeared from review")
	}
	if _, err := applyTaskWorkspace(repository, workspace, base, review.PatchSHA256); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repository, "generated/result.rb")); err != nil {
		t.Fatal("ignored explicit file not accepted")
	}
}
