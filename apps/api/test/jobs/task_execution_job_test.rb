require "test_helper"

class TaskExecutionJobTest < ActiveSupport::TestCase
  include ActiveJob::TestHelper

  test "unexpected model failure becomes a visible failed task" do
    task = make_task
    model = Object.new
    def model.chat(_messages); raise ArgumentError, "invalid model response"; end
    ModelClient.stub(:build, model) { TaskExecutionJob.perform_now(task.id) }
    assert_equal "failed", task.reload.state
    assert_match "invalid model response", task.error_message
  end

  test "concurrent resume is rescheduled instead of lost while previous executor unlocks" do
    task = make_task
    config = Task.connection_db_config.configuration_hash
    # Rails transactional tests may pin checkouts to the same adapter. Use an
    # independent PostgreSQL session so this actually exercises lock contention.
    other_connection = PG.connect(host: config[:host], port: config[:port],
      dbname: config[:database], user: config[:username], password: config[:password])
    namespace = TaskExecutionJob::EXECUTOR_LOCK_NAMESPACE
    other_connection.exec("SELECT pg_advisory_lock(#{namespace}, #{task.id})")
    assert_enqueued_with(job: TaskExecutionJob, args: [task.id]) { TaskExecutionJob.perform_now(task.id) }
    assert_equal "queued", task.reload.state
  ensure
    if other_connection
      other_connection.exec("SELECT pg_advisory_unlock(#{namespace}, #{task.id})")
      other_connection.close
    end
  end

  test "terminal tasks do not invoke agent again" do
    task = make_task(state: "cancelled")
    AgentRunner.stub(:new, ->(*) { flunk "cancelled task must not run" }) { TaskExecutionJob.perform_now(task.id) }
    assert_equal "cancelled", task.reload.state
  end

  test "worker recovery requeues interrupted tasks but leaves approvals alone" do
    queued = make_task
    awaiting = make_task(state: "awaiting_approval")
    unfinished = make_task(state: "created")
    unfinished.update!(workspace_path: nil)
    assert_enqueued_with(job: TaskExecutionJob, args: [queued.id]) { TaskRecovery.enqueue_unfinished }
    assert_equal "awaiting_approval", awaiting.reload.state
    assert_equal "failed", unfinished.reload.state
  end
end
