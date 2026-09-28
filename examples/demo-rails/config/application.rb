require "bundler/setup"
require "rails"
require "action_controller/railtie"

module OvxRailsDemo
  class Application < Rails::Application
    config.load_defaults 8.1
    config.eager_load = false
    config.secret_key_base = "ovx-demo-only-" * 8
    config.hosts.clear
    config.logger = Logger.new($stdout)
  end
end
