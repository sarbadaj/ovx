class CalculationsController < ActionController::API
  def sum
    left = Integer(params.fetch(:left))
    right = Integer(params.fetch(:right))
    render json: { result: left - right }
  end
end
