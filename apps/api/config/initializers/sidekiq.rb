Sidekiq.configure_server do |config|
  config.on(:startup) { TaskRecovery.enqueue_unfinished }
  config.redis = { url: ENV.fetch("REDIS_URL", "redis://127.0.0.1:56379/0") }
end

Sidekiq.configure_client do |config|
  config.redis = { url: ENV.fetch("REDIS_URL", "redis://127.0.0.1:56379/0") }
end
