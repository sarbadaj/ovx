class TaskExecutionJob < ApplicationJob
  queue_as :default

  # A session lock, rather than a long database transaction, lets cancellation
  # update task state immediately while preventing duplicate Sidekiq deliveries
  # from dispatching the same tool concurrently. PostgreSQL releases it on crash.
  EXECUTOR_LOCK_NAMESPACE = 0x4f5658

  def perform(task_id)
    Task.connection_pool.with_connection do |connection|
      locked = connection.select_value("SELECT pg_try_advisory_lock(#{EXECUTOR_LOCK_NAMESPACE}, #{Integer(task_id)})")
      unless locked
        # Approval can be granted before the previous worker has unwound its
        # pause. Preserve that queued resume rather than dropping the delivery.
        self.class.set(wait: 2.seconds).perform_later(task_id)
        return
      end

      begin
        task = Task.find(task_id)
        return if task.terminal? || task.state.in?(%w[awaiting_approval recovery_required])

        AgentRunner.new(task).run
      rescue StandardError => error
        task ||= Task.find_by(id: task_id)
        raise unless task

        task.with_lock do
          unless task.terminal? || task.state.in?(%w[awaiting_approval recovery_required])
            task.update!(state: "failed", error_message: error.message)
            task.agent_events.create!(event_type: "task_failed", payload: { error: error.message })
          end
        end
        Rails.logger.error("OVX task #{task_id} failed: #{error.class}: #{error.message}")
      ensure
        connection.select_value("SELECT pg_advisory_unlock(#{EXECUTOR_LOCK_NAMESPACE}, #{Integer(task_id)})")
      end
    end
  end
end
