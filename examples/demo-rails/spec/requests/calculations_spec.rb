require_relative "../rails_helper"

RSpec.describe "Calculations", type: :request do
  it "adds two integers without changing the JSON contract" do
    get "/sum", params: { left: 2, right: 3 }
    expect(response).to have_http_status(:ok)
    expect(JSON.parse(response.body)).to eq("result" => 5)
  end
end
