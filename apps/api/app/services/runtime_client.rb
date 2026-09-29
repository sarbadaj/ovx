require "net/http"
require "json"

class RuntimeClient
  class RuntimeError < StandardError; end

  def initialize(base_url: ENV.fetch("OVX_RUNTIME_URL", "http://127.0.0.1:7070"))
    @base_url = base_url
    @token = ENV.fetch("OVX_RUNTIME_TOKEN", "")
  end

  def execute(workspace:, tool:, arguments: {})
    post("execute", workspace: workspace, tool: tool, arguments: arguments)
  end

  def register_repository(path:)
    post("repository/register", path: path)
  end

  def create_workspace(repository:, task_id:, task_token:)
    post("workspace/create", repository: repository, task_id: task_id, task_token: task_token)
  end

  def apply_workspace(repository:, workspace:, base_revision:, expected_patch_sha256: nil)
    post("workspace/apply", repository: repository, workspace: workspace, base_revision: base_revision, expected_patch_sha256: expected_patch_sha256)
  end

  def diff_workspace(workspace:, base_revision:)
    post("workspace/diff", workspace: workspace, base_revision: base_revision)
  end

  def cancel_workspace(workspace:)
    post("workspace/cancel", workspace: workspace)
  end

  def resume_workspace(workspace:)
    post("workspace/resume", workspace: workspace)
  end

  def destroy_workspace(workspace:)
    post("workspace/destroy", workspace: workspace)
  end

  private

  def post(path, payload)
    uri = URI.join(@base_url.end_with?("/") ? @base_url : @base_url + "/", path)
    raise RuntimeError, "runtime URL must use HTTP or HTTPS" unless %w[http https].include?(uri.scheme)
    req = Net::HTTP::Post.new(uri)
    req["Content-Type"] = "application/json"
    req["Authorization"] = "Bearer #{@token}" unless @token.empty?
    req.body = JSON.generate(payload)

    http = Net::HTTP.new(uri.host, uri.port)
    http.use_ssl = uri.scheme == "https"
    http.open_timeout = 5
    http.read_timeout = Integer(ENV.fetch("OVX_COMMAND_TIMEOUT_SECONDS", "300")) + 30
    http.write_timeout = 10
    res = http.request(req)
    body = JSON.parse(res.body)
    raise RuntimeError, "runtime response must be an object" unless body.is_a?(Hash)
    raise RuntimeError, body["error"] || "runtime HTTP #{res.code}" unless res.is_a?(Net::HTTPSuccess)
    raise RuntimeError, body["error"].to_s if body["error"].present?

    body
  rescue JSON::ParserError => error
    raise RuntimeError, "runtime returned invalid JSON: #{error.message}"
  rescue IOError, SystemCallError, SocketError, Timeout::Error, OpenSSL::SSL::SSLError => error
    raise RuntimeError, "runtime connection failed: #{error.class}: #{error.message}"
  end
end
