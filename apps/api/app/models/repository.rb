class Repository < ApplicationRecord
  has_many :tasks, dependent: :destroy

  validates :name, :path, presence: true
  validates :path, uniqueness: true
end
