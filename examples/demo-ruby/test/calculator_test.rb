require "minitest/autorun"
require_relative "../lib/calculator"

class CalculatorTest < Minitest::Test
  def test_adds_two_numbers
    assert_equal 5, Calculator.new.add(2, 3)
  end
end
