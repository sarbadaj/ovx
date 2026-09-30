require "test_helper"
require "tmpdir"
require "fileutils"

class LocalApiTest < ActionDispatch::IntegrationTest
  test "health is public and identifies model mode" do
    get "/health"
    assert_response :success
    assert_equal "mock", parsed["model_provider"]
    assert_equal "ovx-api", parsed["service"]
  end

  test "repository access requires bearer auth and rejects browser origins" do
    get "/api/repositories"
    assert_response :unauthorized
    get "/api/repositories", headers: { "Authorization" => ENV.fetch("OVX_API_TOKEN") }
    assert_response :unauthorized
    get "/api/repositories", headers: auth_headers.merge("Origin" => "https://untrusted.example")
    assert_response :forbidden
    get "/api/repositories", headers: auth_headers
    assert_response :success
  end

  test "explicit folder registration allows an external project without Git" do
    Dir.mktmpdir do |project|
      path = File.realpath(project)
      File.write(File.join(path, "Gemfile"), "source 'https://rubygems.org'\n")
      runtime = Minitest::Mock.new
      2.times { runtime.expect(:register_repository, { "path" => path, "registered" => true }, path: path) }
      RuntimeClient.stub(:new, runtime) do
        assert_difference("Repository.count", 1) do
          2.times do
            post "/api/repositories", params: { path: project }, headers: auth_headers, as: :json
            assert_response :created
            assert_equal path, parsed["path"]
          end
        end
      end
      assert_not File.exist?(File.join(path, ".git"))
      runtime.verify
    end
  end

  test "registration accepts a selected Git subfolder with current modified files" do
    Dir.mktmpdir do |project|
      FileUtils.mkdir_p([File.join(project, ".git"), File.join(project, "server")])
      path = File.realpath(File.join(project, "server"))
      File.write(File.join(path, "app.rb"), "uncommitted saved content\n")
      runtime = Minitest::Mock.new
      runtime.expect(:register_repository, { "path" => path, "registered" => true }, path: path)
      RuntimeClient.stub(:new, runtime) do
        post "/api/repositories", params: { path: path }, headers: auth_headers, as: :json
      end
      assert_response :created
      assert_equal path, parsed["path"]
      assert_equal "uncommitted saved content\n", File.read(File.join(path, "app.rb"))
      runtime.verify
    end
  end

  test "runtime private-folder registration rejection never creates a repository" do
    Dir.mktmpdir do |path|
      runtime = Object.new
      def runtime.register_repository(**); raise RuntimeClient::RuntimeError, "OVX private directories cannot be registered"; end
      RuntimeClient.stub(:new, runtime) do
        assert_no_difference("Repository.count") do
          post "/api/repositories", params: { path: path }, headers: auth_headers, as: :json
        end
      end
      assert_response :unprocessable_entity
      assert_match "private directories", parsed["error"]
    end
  end

  test "registering a file or missing folder fails before calling the runtime" do
    Dir.mktmpdir do |directory|
      file = File.join(directory, "file.rb")
      File.write(file, "saved")
      RuntimeClient.stub(:new, -> { flunk "invalid folder must not contact runtime" }) do
        [file, File.join(directory, "missing")].each do |path|
          post "/api/repositories", params: { path: path }, headers: auth_headers, as: :json
          assert_response :unprocessable_entity
        end
      end
    end
  end

  test "task list filters repository and omits full model transcript" do
    first, other = make_task, make_task
    get "/api/tasks", params: { repository_id: first.repository_id }, headers: auth_headers
    assert_response :success
    assert_equal [first.id], parsed.map { |task| task["id"] }
    assert_not parsed.first.key?("transcript")
    get "/api/tasks/#{other.id}", headers: auth_headers
    assert_response :success
    assert parsed.key?("tool_calls")
    assert_not parsed.key?("transcript")
  end

  test "sidebar task response keeps progress without large tool output" do
    task = make_task
    task.agent_events.create!(event_type: "tool_completed", payload: { tool: "read_file", result: { content: "private source" * 10_000 } })
    task.agent_events.create!(event_type: "model_generation_started", payload: { step: 2 })
    get "/api/tasks/#{task.id}", params: { view: "sidebar" }, headers: auth_headers
    assert_response :success
    assert_equal "read_file", parsed["agent_events"][-2].dig("payload", "tool")
    assert_equal "model_generation_started", parsed["agent_events"][-1]["event_type"]
    assert_not response.body.include?("private source")
    assert_not parsed.key?("transcript")
    assert_operator response.body.bytesize, :<, 10_000
  end

  test "create prepares isolated workspace and enqueues job" do
    repo = make_task.repository
    runtime = Minitest::Mock.new
    runtime.expect(:create_workspace, { "workspace" => "/tmp/new-task", "base_revision" => "cafe" }, repository: repo.path, task_id: Integer, task_token: /\A[0-9a-f]{32}\z/)
    RuntimeClient.stub(:new, runtime) do
      assert_enqueued_with(job: TaskExecutionJob) do
        post "/api/tasks", params: { repository_id: repo.id, request: "Fix the test" }, headers: auth_headers, as: :json
      end
    end
    assert_response :accepted
    assert_equal "queued", parsed["state"]
    assert_equal "/tmp/new-task", parsed["workspace_path"]
    assert_not parsed.key?("workspace_token")
    assert_match(/\A[0-9a-f]{32}\z/, Task.find(parsed["id"]).workspace_token)
    runtime.verify
  end

  test "cancellation during workspace creation cannot requeue the task" do
    repo = make_task.repository
    runtime = Object.new
    def runtime.create_workspace(repository:, task_id:, task_token:)
      Task.find(task_id).update!(state: "cancelled")
      { "workspace" => "/tmp/cancelled-clone", "base_revision" => "abc" }
    end
    RuntimeClient.stub(:new, runtime) do
      post "/api/tasks", params: { repository_id: repo.id, request: "Fix" }, headers: auth_headers, as: :json
    end
    assert_response :accepted
    assert_equal "cancelled", parsed["state"]
  end

  test "failed workspace creation remains visible and retryable" do
    repo = make_task.repository
    failing_runtime = Object.new
    def failing_runtime.create_workspace(**); raise RuntimeClient::RuntimeError, "source is dirty"; end
    RuntimeClient.stub(:new, failing_runtime) do
      post "/api/tasks", params: { repository_id: repo.id, request: "Fix" }, headers: auth_headers, as: :json
    end
    assert_response :unprocessable_entity
    assert_equal "failed", parsed["state"]
    assert_match "source is dirty", parsed["error_message"]
  end

  test "diff preserves immutable before and after files for editor" do
    task = make_task(state: "completed")
    snapshot = { "diff" => "patch", "files" => [{ "path" => "lib/calc.rb", "before" => "a-b", "after" => "a+b", "status" => "modified" }] }
    runtime = Minitest::Mock.new
    runtime.expect(:diff_workspace, snapshot, workspace: task.workspace_path, base_revision: task.base_revision)
    RuntimeClient.stub(:new, runtime) { get "/api/tasks/#{task.id}/diff", headers: auth_headers }
    assert_response :success
    assert_equal snapshot, parsed
    runtime.verify
  end

  test "accept is one time and conflicts leave proposal unresolved" do
    task = make_task(state: "completed")
    runtime = Minitest::Mock.new
    runtime.expect(:apply_workspace, { "applied" => true }, repository: task.repository.path, workspace: task.workspace_path, base_revision: task.base_revision, expected_patch_sha256: "abc")
    RuntimeClient.stub(:new, runtime) { post "/api/tasks/#{task.id}/apply_result", params: { expected_patch_sha256: "abc" }, headers: auth_headers, as: :json }
    assert_response :success
    assert_equal "accepted", task.reload.resolution
    post "/api/tasks/#{task.id}/reject_result", headers: auth_headers
    assert_response :conflict
    runtime.verify
  end

  test "runtime apply failure never marks proposal accepted" do
    task = make_task(state: "completed")
    runtime = Object.new
    def runtime.apply_workspace(**); raise RuntimeClient::RuntimeError, "repository changed"; end
    RuntimeClient.stub(:new, runtime) { post "/api/tasks/#{task.id}/apply_result", headers: auth_headers }
    assert_response :unprocessable_entity
    assert_nil task.reload.resolution
  end

  test "reject removes task workspace and persists resolution" do
    task = make_task(state: "completed")
    runtime = Minitest::Mock.new
    runtime.expect(:destroy_workspace, { "destroyed" => true }, workspace: task.workspace_path)
    RuntimeClient.stub(:new, runtime) { post "/api/tasks/#{task.id}/reject_result", headers: auth_headers }
    assert_response :success
    assert_equal "rejected", task.reload.resolution
    get "/api/tasks/#{task.id}/diff", headers: auth_headers
    assert_response :conflict
    runtime.verify
  end

  test "cancellation denies pending approvals and signals runtime" do
    task = make_task(state: "awaiting_approval")
    call = task.tool_calls.create!(tool_name: "run_command", state: "awaiting_approval")
    approval = task.approvals.create!(tool_call: call, status: "pending", reason: "lint")
    runtime = Minitest::Mock.new
    runtime.expect(:cancel_workspace, { "cancelled" => true }, workspace: task.workspace_path)
    RuntimeClient.stub(:new, runtime) { post "/api/tasks/#{task.id}/cancel", headers: auth_headers }
    assert_response :success
    assert_equal "cancelled", task.reload.state
    assert_equal "denied", approval.reload.status
    post "/api/approvals/#{approval.id}/approve", headers: auth_headers
    assert_response :conflict
    runtime.verify
  end

  test "retry resumes failed workspace and queues exactly one job" do
    task = make_task(state: "failed")
    runtime = Minitest::Mock.new
    runtime.expect(:resume_workspace, { "resumed" => true }, workspace: task.workspace_path)
    RuntimeClient.stub(:new, runtime) do
      assert_enqueued_with(job: TaskExecutionJob, args: [task.id]) do
        post "/api/tasks/#{task.id}/retry", headers: auth_headers
      end
      assert_response :accepted
      post "/api/tasks/#{task.id}/retry", headers: auth_headers
      assert_response :conflict
    end
    runtime.verify
  end

  test "approval can be granted once and executes only after authorization" do
    task = make_task(state: "awaiting_approval")
    call = task.tool_calls.create!(tool_name: "run_command", state: "awaiting_approval")
    approval = task.approvals.create!(tool_call: call, reason: "lint")
    assert_enqueued_with(job: TaskExecutionJob, args: [task.id]) do
      post "/api/approvals/#{approval.id}/approve", headers: auth_headers
    end
    assert_response :success
    assert_equal "authorized", call.reload.state
    assert_equal "queued", task.reload.state
    post "/api/approvals/#{approval.id}/deny", headers: auth_headers
    assert_response :conflict
  end
end
