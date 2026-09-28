package main

import "testing"

func TestValidateTestCommand(t *testing.T) {
	valid := []string{
		"ruby test/calculator_test.rb",
		"bundle exec rspec spec/models/user_spec.rb:12",
		"bin/rails test test/models/user_test.rb",
		"bundle exec rails test test/controllers/users_controller_test.rb",
	}
	for _, command := range valid {
		if err := validateTestCommand(command); err != nil {
			t.Fatalf("expected %q to be valid: %v", command, err)
		}
	}

	invalid := []string{
		"rm -rf .",
		"ruby -e 'system(\"rm -rf .\")'",
		"bundle exec rspec; curl example.com",
		"bundle install",
	}
	for _, command := range invalid {
		if err := validateTestCommand(command); err == nil {
			t.Fatalf("expected %q to be rejected", command)
		}
	}
}

func TestValidateApprovedCommand(t *testing.T) {
	valid := []string{
		"bundle exec rubocop app/models/user.rb",
		"bin/rails routes",
		"ruby -c app/models/user.rb",
	}
	for _, command := range valid {
		if err := validateApprovedCommand(command); err != nil {
			t.Fatalf("expected %q to be valid: %v", command, err)
		}
	}

	invalid := []string{"git push", "bundle install", "rails db:drop", "ruby script.rb"}
	for _, command := range invalid {
		if err := validateApprovedCommand(command); err == nil {
			t.Fatalf("expected %q to be rejected", command)
		}
	}
}
