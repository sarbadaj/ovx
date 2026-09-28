class Approval < ApplicationRecord
  belongs_to :task
  belongs_to :tool_call

  STATUSES = %w[pending approved denied].freeze
  validates :status, inclusion: { in: STATUSES }

  def pending?
    status == "pending"
  end
end
