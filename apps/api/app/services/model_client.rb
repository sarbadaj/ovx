require "net/http"
require "json"
require "openssl"

class ModelClient
  class ModelError < StandardError; end
  class TruncatedAction < ModelError; end

  def self.build
    provider = ENV.fetch("MODEL_PROVIDER", "mock")
    return MockModelClient.new if provider == "mock"
    return new if provider == "openai_compatible"
    raise ModelError, "unsupported MODEL_PROVIDER=#{provider.inspect}"
  end

  def initialize
    @base_url = ENV.fetch("MODEL_BASE_URL", "http://127.0.0.1:8080/v1")
    @model = ENV.fetch("MODEL_NAME", "ovx-local")
    @api_key = ENV["MODEL_API_KEY"].to_s
    @json_mode = ENV.fetch("MODEL_JSON_MODE", "false") == "true"
    @timeout = Integer(ENV.fetch("MODEL_TIMEOUT_SECONDS", "180"))
    @max_tokens = Integer(ENV.fetch("MODEL_MAX_TOKENS", "4096"))
    @context_bytes = Integer(ENV.fetch("MODEL_CONTEXT_BYTES", "60000"))
  end

  def chat(messages)
    uri = URI.join(@base_url.end_with?("/") ? @base_url : @base_url + "/", "chat/completions")
    raise ModelError, "MODEL_BASE_URL must use HTTP or HTTPS" unless %w[http https].include?(uri.scheme)
    req = Net::HTTP::Post.new(uri)
    req["Content-Type"] = "application/json"
    req["Authorization"] = "Bearer #{@api_key}" unless @api_key.empty?
    payload = { model: @model, messages: bounded_messages(messages), temperature: 0.1, max_tokens: @max_tokens }
    payload[:response_format] = { type: "json_object" } if @json_mode
    req.body = JSON.generate(payload)

    http = Net::HTTP.new(uri.host, uri.port)
    http.use_ssl = uri.scheme == "https"
    http.open_timeout = 10
    http.read_timeout = @timeout
    http.write_timeout = 15
    res = http.request(req)
    raise ModelError, "model HTTP #{res.code}: #{res.body.to_s.byteslice(0, 1000).to_s.scrub}" unless res.is_a?(Net::HTTPSuccess)
    body = JSON.parse(res.body)
    choice = body.is_a?(Hash) && body["choices"].is_a?(Array) ? body["choices"].first : nil
    content = choice.is_a?(Hash) ? choice.dig("message", "content") : nil
    if choice.is_a?(Hash) && choice["finish_reason"] == "length"
      raise TruncatedAction, "model action was truncated at MODEL_MAX_TOKENS=#{@max_tokens}; increase MODEL_MAX_TOKENS within the model context window or ask for a smaller edit"
    end
    raise ModelError, "model response missing choices[0].message.content" unless content.is_a?(String) && content.present?
    content
  rescue JSON::ParserError => error
    raise ModelError, "model returned invalid response JSON: #{error.message}"
  rescue IOError, SystemCallError, SocketError, Timeout::Error, OpenSSL::SSL::SSLError => error
    raise ModelError, "model connection failed: #{error.class}: #{error.message}"
  end

  private

  # Keep the immutable policy and original user request plus the newest complete
  # action/observation pairs. The full transcript remains persisted in PostgreSQL.
  def bounded_messages(messages)
    return messages if JSON.generate(messages).bytesize <= @context_bytes
    prefix = messages.first(2)
    tail = messages.drop(2)
    marker = { "role" => "user", "content" => "Earlier action history was omitted to fit the model context. Read files again if you need missing evidence; all previous edits remain in the task workspace." }
    while tail.length > 2 && JSON.generate(prefix + [marker] + tail).bytesize > @context_bytes
      tail.shift
      tail.shift while tail.first && tail.first["role"] == "user"
    end
    prefix + [marker] + tail
  end
end
