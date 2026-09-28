Rails.application.configure do
  config.enable_reloading = false
  config.active_job.queue_adapter = :test
  config.hosts.clear
  config.eager_load = ENV["CI"].present?
  config.consider_all_requests_local = true
end
