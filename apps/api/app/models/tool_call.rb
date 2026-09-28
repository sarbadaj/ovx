class ToolCall < ApplicationRecord
  belongs_to :task
  has_many :tool_attempts, -> { order(:id) }, dependent: :destroy
  has_one :approval, dependent: :destroy

  STATES = %w[requested awaiting_approval authorized started completed failed cancelled unknown].freeze
  validates :tool_name, presence: true
  validates :state, inclusion: { in: STATES }
end
