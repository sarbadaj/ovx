module Api
  class ApprovalsController < ApplicationController
    def index
      scope = Approval.includes(:tool_call).order(:id)
      scope = scope.where(status: params[:status]) if params[:status].present?
      render json: scope.as_json(include: :tool_call)
    end

    def show
      render json: Approval.find(params[:id]).as_json(include: :tool_call)
    end

    def approve
      decide("approved")
    end

    def deny
      decide("denied")
    end

    private

    def decide(decision)
      approval = Approval.find(params[:id])
      task = approval.task
      task.with_lock do
        approval.reload
        unless approval.pending? && task.state == "awaiting_approval"
          return render json: { error: "approval is already decided or task is no longer awaiting approval" }, status: :conflict
        end
        approval.update!(status: decision, decided_at: Time.current)
        approval.tool_call.update!(state: decision == "approved" ? "authorized" : "cancelled")
        task.update!(state: decision == "approved" ? "queued" : "failed", error_message: decision == "denied" ? "user denied required approval" : nil)
        if decision == "denied"
          transcript = task.transcript + [{ "role" => "user", "content" => "The user denied #{approval.tool_call.tool_name}. Do not repeat that action; choose a safe alternative if retried." }]
          task.update!(transcript: transcript)
        end
        task.agent_events.create!(event_type: decision == "approved" ? "approval_granted" : "approval_denied", payload: { approval_id: approval.id, tool_call_id: approval.tool_call_id })
      end
      TaskExecutionJob.perform_later(task.id) if decision == "approved"
      render json: approval.reload
    rescue ActiveJob::EnqueueError, RedisClient::Error => error
      task.with_lock do
        task.update!(state: "failed", error_message: "Could not enqueue approved task: #{error.message}") unless task.terminal?
      end
      render json: { error: error.message }, status: :service_unavailable
    end
  end
end
