require "test_helper"

class AgentRunnerTest < ActiveSupport::TestCase
  class ScriptedModel
    def initialize(*actions, &block)
      @actions, @block = actions, block
    end
    def chat(messages)
      @block&.call(messages)
      action = @actions.shift
      raise "script exhausted" unless action
      action.is_a?(String) ? action : JSON.generate(action)
    end
  end

  class FakeRuntime
    attr_reader :calls, :commands
    def initialize(changed: true, exit_codes: [0])
      @changed, @exit_codes, @calls, @commands = changed, exit_codes, [], []
    end
    def execute(workspace:, tool:, arguments:)
      @calls << tool
      @commands << arguments["command"] if tool == "run_tests"
      { "output" => "ok", "exit_code" => tool == "run_tests" ? (@exit_codes.shift || 0) : 0 }
    end
    def diff_workspace(**)
      { "files" => @changed ? [{ "path" => "calc.rb" }] : [] }
    end
  end

  def tool(name)
    { type: "tool", tool: name, arguments: {} }
  end

  def finish
    { type: "final", message: "Fixed and tested" }
  end

  test "unsupported dependency installation does not create a dead end approval" do
    task = make_task
    model = ScriptedModel.new({ type: 'tool', tool: 'run_command', arguments: { command: 'bundle install' } }, finish)
    AgentRunner.new(task, model: model, runtime: FakeRuntime.new(changed: false)).run
    assert_empty task.approvals
    assert_empty task.tool_calls
    assert task.transcript.any? { |message| message['content'].include?('bundle install is unavailable') }
  end

  test "replacement requests cannot turn into file deletion approvals" do
    task = make_task
    model = ScriptedModel.new({ type: 'tool', tool: 'delete_file', arguments: { path: 'lib/project.rb', reason: 'replace with updated code' } }, finish)
    AgentRunner.new(task, model: model, runtime: FakeRuntime.new(changed: false)).run
    assert_empty task.approvals
    assert_empty task.tool_calls
    assert task.transcript.any? { |message| message['content'].include?('Do not delete a file to change it') }
  end

  test "repeating a rejected replacement stops with a useful error" do
    task = make_task
    action = { type: 'tool', tool: 'delete_file', arguments: { path: 'lib/project.rb', reason: 'replace file' } }
    error = assert_raises(AgentRunner::AgentError) do
      AgentRunner.new(task, model: ScriptedModel.new(action, action, action), runtime: FakeRuntime.new(changed: false)).run
    end
    assert_match 'repeated an invalid delete_file action', error.message
    assert_empty task.approvals
  end

  test "missing sandbox gems stops with environment setup instructions and preserved output" do
    task = make_task
    runtime = FakeRuntime.new
    def runtime.execute(**)
      { 'output' => 'Bundler::GemNotFound: missing project gems', 'exit_code' => 1 }
    end
    error = assert_raises(AgentRunner::AgentError) do
      AgentRunner.new(task, model: ScriptedModel.new(tool('run_tests')), runtime: runtime).run
    end
    assert_match 'ovx_prepare_rails.py', error.message
    assert_equal 1, task.tool_calls.last.tool_attempts.last.exit_code
    assert_empty task.approvals
  end

  test "repeated failed path is replaced with repository discovery" do
    task = make_task
    runtime = FakeRuntime.new(changed: false)
    def runtime.execute(workspace:, tool:, arguments:)
      @calls << tool
      raise RuntimeClient::RuntimeError, 'file does not exist' if tool == 'read_file'
      { 'output' => ['server/spec/example_spec.rb'], 'exit_code' => 0 }
    end
    missing = { type: 'tool', tool: 'read_file', arguments: { path: 'spec/example_spec.rb' } }
    AgentRunner.new(task, model: ScriptedModel.new(missing, missing, finish), runtime: runtime).run
    assert_equal ['read_file', 'list_files'], runtime.calls
    assert task.transcript.any? { |message| message['content'].include?('server/spec/example_spec.rb') }
  end

  test "ambiguous patch failure tells the model to use unique context" do
    task = make_task
    runtime = FakeRuntime.new(changed: false)
    def runtime.execute(workspace:, tool:, arguments:)
      raise RuntimeClient::RuntimeError, 'old text must match exactly once; matched 2 times' if tool == 'apply_patch'
      super
    end
    AgentRunner.new(task, model: ScriptedModel.new(tool('apply_patch'), tool('run_tests'), tool('git_diff'), finish), runtime: runtime).run
    assert task.transcript.any? { |message| message['content'].include?('Include surrounding lines') }
  end

  test "truncated responses are discarded and retried with smaller action guidance" do
    task = make_task
    calls = 0
    model = ScriptedModel.new(finish) do
      calls += 1
      raise ModelClient::TruncatedAction, 'output budget exceeded' if calls == 1
    end
    AgentRunner.new(task, model: model, runtime: FakeRuntime.new(changed: false)).run
    assert_equal 'completed', task.reload.state
    assert_equal 2, calls
    assert task.agent_events.where(event_type: 'model_response_retry').exists?
    assert_empty task.tool_calls
  end

  test "completion requires passing tests and diff after latest edit" do
    task = make_task
    runtime = FakeRuntime.new
    model = ScriptedModel.new(tool("apply_patch"), tool("run_tests"), tool("git_diff"), tool("apply_patch"), finish,
      tool("run_tests"), finish, tool("git_diff"), finish)
    AgentRunner.new(task, model: model, runtime: runtime).run
    assert_equal "completed", task.reload.state
    assert_equal 2, runtime.calls.count("run_tests")
    assert task.transcript.any? { |message| message["content"].include?("Earlier passing tests") }
    assert task.transcript.any? { |message| message["content"].include?("Inspect git_diff after") }
  end

  test "latest failing test invalidates earlier passing test" do
    task = make_task
    runtime = FakeRuntime.new(exit_codes: [0, 1, 0])
    model = ScriptedModel.new(tool("apply_patch"), tool("run_tests"), tool("run_tests"), tool("git_diff"), finish,
      tool("run_tests"), tool("git_diff"), finish)
    AgentRunner.new(task, model: model, runtime: runtime).run
    assert_equal "completed", task.reload.state
    assert_equal 3, runtime.calls.count("run_tests")
  end

  test "a syntax check cannot validate a changed test file" do
    task = make_task
    runtime = FakeRuntime.new
    def runtime.diff_workspace(**)
      { "files" => [{ "path" => "test/project_test.rb" }, { "path" => "test/other_test.rb" }] }
    end
    syntax = { type: "tool", tool: "run_tests", arguments: { command: "ruby -c lib/project.rb" } }
    suite = { type: "tool", tool: "run_tests", arguments: { command: "ruby test/project_test.rb" } }
    AgentRunner.new(task, model: ScriptedModel.new(tool("apply_patch"), syntax, tool("git_diff"), finish, suite, tool("git_diff"), finish), runtime: runtime).run
    assert_equal "completed", task.reload.state
    assert task.transcript.any? { |message| message["content"].include?("syntax check is insufficient") }
    assert_equal 2, runtime.calls.count("run_tests")
  end

  test "a changed Ruby test converts a syntax-only check into a functional test run" do
    task = make_task
    runtime = FakeRuntime.new
    def runtime.diff_workspace(**)
      { "files" => [{ "path" => "test/project_test.rb" }] }
    end
    syntax = { type: "tool", tool: "run_tests", arguments: { command: "ruby -c lib/project.rb" } }
    AgentRunner.new(task, model: ScriptedModel.new(tool("apply_patch"), syntax, tool("git_diff"), finish), runtime: runtime).run
    assert_equal ["ruby test/project_test.rb"], runtime.commands
    assert_equal "completed", task.reload.state
  end

  test "a failed test cannot be declared completed just because no files changed" do
    task = make_task
    runtime = FakeRuntime.new(changed: false, exit_codes: [1, 0])
    model = ScriptedModel.new(tool('run_tests'), finish, tool('run_tests'), tool('git_diff'), finish)
    AgentRunner.new(task, model: model, runtime: runtime).run
    assert_equal 2, runtime.calls.count('run_tests')
    assert_equal 'completed', task.reload.state
  end

  test "model final with no edits can finish without fake test claims enforced by edit rule" do
    task = make_task
    AgentRunner.new(task, model: ScriptedModel.new(finish), runtime: FakeRuntime.new(changed: false)).run
    assert_equal "completed", task.reload.state
  end

  test "cancel during model generation does not execute its returned tool" do
    task = make_task
    runtime = FakeRuntime.new
    model = ScriptedModel.new(tool("apply_patch")) { task.update!(state: "cancelled") }
    AgentRunner.new(task, model: model, runtime: runtime).run
    assert_equal "cancelled", task.reload.state
    assert_empty runtime.calls
    assert_empty task.tool_calls
  end

  test "delete file pauses for explicit approval before execution" do
    task = make_task
    runtime = FakeRuntime.new
    AgentRunner.new(task, model: ScriptedModel.new(tool("delete_file")), runtime: runtime).run
    assert_equal "awaiting_approval", task.reload.state
    assert_equal "pending", task.approvals.first.status
    assert_empty runtime.calls
  end

  test "authorized command resumes once before further model calls" do
    task = make_task
    call = task.tool_calls.create!(tool_name: "run_command", state: "authorized", arguments: { command: "bundle exec rubocop" })
    runtime = FakeRuntime.new
    AgentRunner.new(task, model: ScriptedModel.new(tool("run_tests"), tool("git_diff"), finish), runtime: runtime).run
    assert_equal "completed", task.reload.state
    assert_equal "run_command", runtime.calls.first
    assert_equal 1, call.tool_attempts.count
  end

  test "stale executing tool requires recovery instead of replay" do
    task = make_task
    call = task.tool_calls.create!(tool_name: "apply_patch", state: "started")
    attempt = call.tool_attempts.create!(state: "started")
    runtime = FakeRuntime.new
    AgentRunner.new(task, model: ScriptedModel.new(finish), runtime: runtime).run
    assert_equal "recovery_required", task.reload.state
    assert_equal "unknown", attempt.reload.state
    assert_empty runtime.calls
  end

  test "malformed model response gets bounded correction opportunity" do
    task = make_task
    AgentRunner.new(task, model: ScriptedModel.new("not json", finish), runtime: FakeRuntime.new(changed: false)).run
    assert_equal "completed", task.reload.state
    assert task.transcript.any? { |message| message["content"].include?("Invalid action") }
  end
end
