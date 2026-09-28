require "test_helper"
require "net/http"

class ModelClientTest < ActiveSupport::TestCase
  class FakeHTTP
    attr_accessor :use_ssl, :open_timeout, :read_timeout, :write_timeout
    attr_reader :sent
    def initialize(body)
      @body = body
    end
    def request(request)
      @sent = JSON.parse(request.body)
      response = Net::HTTPOK.new("1.1", "200", "OK")
      response.define_singleton_method(:body) { JSON.generate(@fixture) }
      response.instance_variable_set(:@fixture, @body)
      response
    end
  end

  test "OpenAI compatible adapter sends JSON mode and receives model content" do
    previous = ENV["MODEL_JSON_MODE"]
    previous_tokens = ENV["MODEL_MAX_TOKENS"]
    ENV["MODEL_JSON_MODE"] = "true"
    ENV["MODEL_MAX_TOKENS"] = "777"
    http = FakeHTTP.new({ "choices" => [{ "message" => { "content" => '{"type":"final","message":"done"}' } }] })
    Net::HTTP.stub(:new, http) do
      assert_match '"final"', ModelClient.new.chat([{ "role" => "user", "content" => "Fix" }])
    end
    assert_equal({ "type" => "json_object" }, http.sent["response_format"])
    assert_equal 777, http.sent["max_tokens"]
  ensure
    ENV["MODEL_JSON_MODE"] = previous
    ENV["MODEL_MAX_TOKENS"] = previous_tokens
  end

  test "truncated generation is a visible retryable failure" do
    http = FakeHTTP.new({ "choices" => [{ "finish_reason" => "length", "message" => { "content" => "partial JSON" } }] })
    Net::HTTP.stub(:new, http) do
      error = assert_raises(ModelClient::ModelError) { ModelClient.new.chat([]) }
      assert_match "MODEL_MAX_TOKENS", error.message
    end
  end

  test "missing choices are rejected with a useful error" do
    Net::HTTP.stub(:new, FakeHTTP.new({ "result" => [] })) do
      assert_raises(ModelClient::ModelError) { ModelClient.new.chat([]) }
    end
  end

  test "context budgeting retains policy original request and recent evidence" do
    previous = ENV["MODEL_CONTEXT_BYTES"]
    ENV["MODEL_CONTEXT_BYTES"] = "1200"
    messages = [{ "role" => "system", "content" => "policy" }, { "role" => "user", "content" => "original task" }]
    20.times do |index|
      messages << { "role" => "assistant", "content" => "action #{index}" }
      messages << { "role" => "user", "content" => "evidence #{index} " + "x" * 100 }
    end
    http = FakeHTTP.new({ "choices" => [{ "message" => { "content" => "ok" } }] })
    Net::HTTP.stub(:new, http) { ModelClient.new.chat(messages) }
    sent = http.sent["messages"]
    assert_equal messages.first(2), sent.first(2)
    assert_equal messages.last(2), sent.last(2)
    assert_operator sent.length, :<, messages.length
    assert_operator JSON.generate(sent).bytesize, :<=, 1200
  ensure
    ENV["MODEL_CONTEXT_BYTES"] = previous
  end
end
