class Task < ApplicationRecord
  before_validation :assign_workspace_token, on: :create
  belongs_to :repository
  has_many :agent_events, -> { order(:id) }, dependent: :destroy
  has_many :tool_calls, -> { order(:id) }, dependent: :destroy
  has_many :approvals, -> { order(:id) }, dependent: :destroy

  STATES = %w[
    created queued planning executing validating awaiting_approval
    recovery_required completed failed cancelled
  ].freeze

  validates :user_request, presence: true, length: { maximum: 32_000 }
  validates :resolution, inclusion: { in: %w[accepted rejected] }, allow_nil: true
  validates :state, inclusion: { in: STATES }

  def terminal?
    state.in?(%w[completed failed cancelled])
  end

  private

  def assign_workspace_token
    self.workspace_token ||= SecureRandom.hex(16)
  end
end
