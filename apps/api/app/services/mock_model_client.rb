require "json"

# Deterministic provider used only for the included smoke-test repository.
# It proves that Rails -> Sidekiq -> runtime -> sandbox -> persistence works
# before the developer downloads a real local model.
class MockModelClient
  def chat(messages)
    all = messages.map { |m| m[:content] || m["content"] }.join("
")
    tool_results = all.scan(/TOOL RESULT /).length
    planned = all.include?("Plan accepted")

    return JSON.generate(type: "plan", steps: [
      "Inspect repository files",
      "Read implementation and test",
      "Apply the smallest code fix",
      "Run the targeted test",
      "Inspect the Git diff"
    ]) unless planned

    case tool_results
    when 0
      JSON.generate(type: "tool", tool: "list_files", arguments: {})
    when 1
      JSON.generate(type: "tool", tool: "read_file", arguments: { path: "lib/calculator.rb" })
    when 2
      JSON.generate(type: "tool", tool: "read_file", arguments: { path: "test/calculator_test.rb" })
    when 3
      JSON.generate(type: "tool", tool: "apply_patch", arguments: {
        path: "lib/calculator.rb", old: "a - b", new: "a + b"
      })
    when 4
      JSON.generate(type: "tool", tool: "run_tests", arguments: {
        command: "ruby test/calculator_test.rb"
      })
    when 5
      JSON.generate(type: "tool", tool: "git_diff", arguments: {})
    else
      JSON.generate(type: "final", message: "Fixed Calculator#add, ran the targeted test successfully, and inspected the Git diff.")
    end
  end
end
