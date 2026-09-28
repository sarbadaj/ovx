# Rails request-spec fixture

A small real Rails application with an intentionally failing RSpec request spec.
`GET /sum?left=2&right=3` must return `{"result":5}`. The implementation currently
subtracts. Ask OVX to fix the failing spec without modifying tests or the API.

This fixture uses Action Controller and needs no external database or network.
Its dependency image is built with `docker/sandbox-demo-rails.Dockerfile` from
the OVX repository root. Run `bundle exec rspec` inside that sandbox.
