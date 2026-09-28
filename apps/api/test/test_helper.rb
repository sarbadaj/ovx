ENV["RAILS_ENV"] ||= "test"
ENV["OVX_API_TOKEN"] = "ovx-test-token-that-is-not-a-real-secret"
ENV["MODEL_PROVIDER"] = "mock"
require_relative "../config/environment"
require "rails/test_help"
require "minitest/mock"

class ActiveSupport::TestCase
  def make_task(state: "queued", **attributes)
    repository = Repository.create!(name: "demo", path: "/tmp/ovx-test-#{SecureRandom.hex(8)}")
    repository.tasks.create!(user_request: "Fix the calculator", state: state, workspace_path: "/tmp/ovx-task", base_revision: "abc123", **attributes)
  end
end

class ActionDispatch::IntegrationTest
  include ActiveJob::TestHelper

  def auth_headers
    { "Authorization" => "Bearer #{ENV.fetch('OVX_API_TOKEN')}" }
  end

  def parsed
    response.parsed_body
  end
end
