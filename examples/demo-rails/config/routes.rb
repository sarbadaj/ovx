Rails.application.routes.draw do
  get "/sum", to: "calculations#sum"
end
