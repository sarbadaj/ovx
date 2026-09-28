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
    attr_reader :calls
    def initialize(changed: true, exit_codes: [0])
      @changed, @exit_codes, @calls = changed, exit_codes, []
    end
    def execute(workspace:, tool:, arguments:)
      @calls << tool
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
