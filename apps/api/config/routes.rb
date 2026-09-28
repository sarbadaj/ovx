Rails.application.routes.draw do
  get "/health", to: proc { |_env|
    payload = { status: "ok", service: "ovx-api", model_provider: ENV.fetch("MODEL_PROVIDER", "mock"), model_name: ENV.fetch("MODEL_NAME", "ovx-local") }
    [200, { "content-type" => "application/json" }, [JSON.generate(payload)]]
  }

  namespace :api do
    resources :repositories, only: %i[index show create]
    resources :tasks, only: %i[index create show] do
      member do
        get :events
        get :diff
        post :cancel
        post :retry
        post :apply_result
        post :reject_result
      end
    end
    resources :approvals, only: %i[index show] do
      member do
        post :approve
        post :deny
      end
    end
  end
end
