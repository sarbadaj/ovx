module Api
  class TasksController < ApplicationController
    def index
      tasks = Task.order(id: :desc)
      tasks = tasks.where(repository_id: params[:repository_id]) if params[:repository_id].present?
      render json: tasks.limit(100).as_json(except: %i[transcript workspace_token])
    end

    def show
      render json: task_payload(Task.find(params[:id]))
    end

    def events
      scope = Task.find(params[:id]).agent_events.order(:id)
      scope = scope.where("id > ?", Integer(params[:after_id])) if params[:after_id].present?
      render json: scope
    end

    def diff
      task = Task.find(params[:id])
      if task.workspace_path.blank? || task.resolution == "rejected"
        return render json: { error: "task workspace is unavailable" }, status: :conflict
      end
      render json: RuntimeClient.new.diff_workspace(workspace: task.workspace_path, base_revision: task.base_revision)
    end

    def create
      repository = Repository.find(params.require(:repository_id))
      task = repository.tasks.create!(user_request: params.require(:request), state: "created", transcript: [])
      task.agent_events.create!(event_type: "task_created", payload: { request: task.user_request })
      prepare_workspace!(task)
      TaskExecutionJob.perform_later(task.id)
      render json: task_payload(task), status: :accepted
    rescue RuntimeClient::RuntimeError, ActiveJob::EnqueueError, RedisClient::Error => error
      fail_task!(task, error)
      render json: task ? task_payload(task) : { error: error.message }, status: :unprocessable_entity
    end

    def cancel
      task = Task.find(params[:id])
      task.with_lock do
        if task.terminal?
          return render json: { error: "task is already terminal" }, status: :conflict
        end
        task.approvals.where(status: "pending").find_each do |approval|
          approval.update!(status: "denied", decided_at: Time.current)
          approval.tool_call.update!(state: "cancelled")
        end
        task.tool_calls.where(state: %w[requested authorized]).update_all(state: "cancelled", completed_at: Time.current)
        task.update!(state: "cancelled")
        task.agent_events.create!(event_type: "task_cancelled", payload: {})
      end
      begin
        RuntimeClient.new.cancel_workspace(workspace: task.workspace_path) if task.workspace_path.present?
      rescue RuntimeClient::RuntimeError => error
        task.agent_events.create!(event_type: "cancellation_warning", payload: { error: error.message })
      end
      render json: task_payload(task)
    end

    def retry
      task = Task.find(params[:id])
      task.with_lock do
        unless task.state.in?(%w[failed recovery_required]) && task.resolution.blank?
          return render json: { error: "only unresolved failed or recovery_required tasks can be retried" }, status: :conflict
        end
        if task.workspace_path.blank?
          prepare_workspace!(task)
        else
          RuntimeClient.new.resume_workspace(workspace: task.workspace_path)
          task.update!(state: "queued", error_message: nil)
        end
        task.agent_events.create!(event_type: "task_retry_queued", payload: {})
      end
      TaskExecutionJob.perform_later(task.id)
      render json: task_payload(task), status: :accepted
    rescue ActiveJob::EnqueueError, RedisClient::Error => error
      fail_task!(task, error)
      render json: { error: error.message }, status: :service_unavailable
    end

    def apply_result
      task = Task.find(params[:id])
      task.with_lock do
        return unless unresolved_result?(task)

        result = RuntimeClient.new.apply_workspace(
          repository: task.repository.path, workspace: task.workspace_path, base_revision: task.base_revision,
          expected_patch_sha256: params[:expected_patch_sha256].presence
        )
        task.update!(resolution: "accepted", applied_at: Time.current)
        task.agent_events.create!(event_type: "task_patch_applied", payload: result)
      end
      render json: task_payload(task)
    end

    def reject_result
      task = Task.find(params[:id])
      task.with_lock do
        return unless unresolved_result?(task)

        RuntimeClient.new.destroy_workspace(workspace: task.workspace_path)
        task.update!(resolution: "rejected")
        task.agent_events.create!(event_type: "task_result_rejected", payload: {})
      end
      render json: task_payload(task)
    end

    private

    def unresolved_result?(task)
      if task.state != "completed"
        render json: { error: "task must be completed before accepting or rejecting" }, status: :conflict
        return false
      end
      if task.resolution.present?
        render json: { error: "task already resolved as #{task.resolution}" }, status: :conflict
        return false
      end
      true
    end

    def prepare_workspace!(task)
      task.update!(workspace_token: SecureRandom.hex(16)) if task.workspace_token.blank?
      workspace = RuntimeClient.new.create_workspace(repository: task.repository.path, task_id: task.id, task_token: task.workspace_token)
      task.with_lock do
        # A client can cancel from history while cloning is still in flight.
        task.update!(workspace_path: workspace.fetch("workspace"), base_revision: workspace.fetch("base_revision"))
        task.update!(state: "queued", error_message: nil) unless task.terminal?
        task.agent_events.create!(event_type: "workspace_created", payload: workspace)
      end
    end

    def fail_task!(task, error)
      return unless task
      task.with_lock do
        return if task.terminal?
        task.update!(state: "failed", error_message: error.message)
        task.agent_events.create!(event_type: "task_failed", payload: { error: error.message })
      end
    end

    def task_payload(task)
      return sidebar_task_payload(task) if params[:view] == "sidebar"

      task.reload.as_json(except: %i[transcript workspace_token], include: {
        agent_events: {}, tool_calls: { include: :tool_attempts }, approvals: {}
      })
    end

    def sidebar_task_payload(task)
      task.reload
      approvals = task.approvals.where(status: "pending").to_a
      calls = task.tool_calls.where(tool_name: "run_tests").order(id: :desc).limit(20).to_a
      calls.concat(task.tool_calls.where(id: approvals.map(&:tool_call_id)).to_a)
      {
        id: task.id, repository_id: task.repository_id, user_request: task.user_request,
        state: task.state, resolution: task.resolution, workspace_path: task.workspace_path,
        base_revision: task.base_revision, plan: task.plan, final_message: task.final_message,
        error_message: task.error_message, created_at: task.created_at, updated_at: task.updated_at,
        agent_events: task.agent_events.reorder(id: :desc).limit(80).to_a.reverse.map do |event|
          payload = event.payload || {}
          { id: event.id, event_type: event.event_type, created_at: event.created_at,
            payload: payload.slice("step", "tool", "state", "reason", "error", "message").transform_values { |value| value.to_s.byteslice(0, 500).to_s.scrub } }
        end,
        tool_calls: calls.uniq(&:id).map do |call|
          { id: call.id, tool_name: call.tool_name, arguments: call.arguments,
            tool_attempts: call.tool_attempts.map do |attempt|
              result = attempt.result || {}
              output = result["output"].is_a?(Hash) ? result["output"] : result
              output = {} unless output.is_a?(Hash)
              { id: attempt.id, state: attempt.state, exit_code: attempt.exit_code,
                started_at: attempt.started_at, finished_at: attempt.finished_at,
                error_message: attempt.error_message,
                result: { stdout: output["stdout"].to_s.byteslice(0, 4_000).to_s.scrub,
                          stderr: output["stderr"].to_s.byteslice(0, 4_000).to_s.scrub } }
            end }
        end,
        approvals: approvals.as_json
      }
    end
  end
end
