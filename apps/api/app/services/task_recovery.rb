# Requeue unfinished local work after an unclean worker shutdown. AgentRunner
# reconciles any started tool as unknown before attempting further execution.
class TaskRecovery
  def self.enqueue_unfinished
    Task.where(state: %w[created queued planning executing validating]).find_each do |task|
      if task.workspace_path.blank?
        task.with_lock do
          next if task.workspace_path.present? || task.terminal?
          task.update!(state: "failed", error_message: "workspace preparation was interrupted; retry to prepare it again")
          task.agent_events.create!(event_type: "task_failed", payload: { error: task.error_message })
        end
      else
        TaskExecutionJob.perform_later(task.id)
      end
    end
  end
end
