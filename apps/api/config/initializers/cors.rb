Rails.application.config.middleware.insert_before 0, Rack::Cors do
  allow do
    origins *ENV.fetch("OVX_CORS_ORIGINS", "").split(",")
    resource "*", headers: :any, methods: %i[get post options]
  end
end
