class ToolAttempt < ApplicationRecord
  belongs_to :tool_call

  STATES = %w[started completed failed cancelled unknown].freeze
  validates :state, inclusion: { in: STATES }
end
