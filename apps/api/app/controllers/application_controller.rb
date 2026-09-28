class ApplicationController < ActionController::API
  before_action :authenticate_local_client!

  rescue_from ActiveRecord::RecordNotFound do |error|
    render json: { error: error.message }, status: :not_found
  end

  rescue_from ActionController::ParameterMissing, ActiveRecord::RecordInvalid, ArgumentError do |error|
    render json: { error: error.message }, status: :unprocessable_entity
  end

  rescue_from RuntimeClient::RuntimeError do |error|
    render json: { error: error.message }, status: :unprocessable_entity
  end

  private

  def authenticate_local_client!
    expected = ENV["OVX_API_TOKEN"].to_s
    if expected.empty?
      render json: { error: "OVX_API_TOKEN is not configured; run bin/ovx start" }, status: :service_unavailable
      return
    end

    # Browser pages must never control the localhost agent, including when a
    # user accidentally pastes their token into an unrelated website.
    origin = request.headers["Origin"]
    allowed_origins = ENV.fetch("OVX_CORS_ORIGINS", "").split(",")
    if origin.present? && !allowed_origins.include?(origin)
      render json: { error: "origin is not allowed" }, status: :forbidden
      return
    end

    header = request.headers["Authorization"].to_s
    supplied = header.start_with?("Bearer ") ? header.delete_prefix("Bearer ") : ""
    unless ActiveSupport::SecurityUtils.secure_compare(supplied, expected)
      response.set_header("WWW-Authenticate", "Bearer")
      render json: { error: "invalid or missing OVX API token" }, status: :unauthorized
    end
  end
end
