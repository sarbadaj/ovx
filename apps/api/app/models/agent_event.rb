class AgentEvent < ApplicationRecord
  belongs_to :task

  validates :event_type, presence: true
end
