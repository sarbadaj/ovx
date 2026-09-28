Rails.application.configure do
  raise "DATABASE_URL must be set in production" if ENV["DATABASE_URL"].blank?
  config.enable_reloading = false
  config.eager_load = true
  config.consider_all_requests_local = false
  config.force_ssl = ENV.fetch("FORCE_SSL", "false") == "true"
end
