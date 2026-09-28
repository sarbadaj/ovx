# Copy this file into the target Rails repository and customize it.
FROM ruby:3.3-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       build-essential git libpq-dev libsqlite3-dev sqlite3 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
COPY Gemfile Gemfile.lock /tmp/ovx-app/
WORKDIR /tmp/ovx-app
RUN bundle config set --global frozen true && bundle install
# Use a test database reachable entirely inside this network-disabled container.
# For SQLite tests, configure a writable path in /tmp or /workspace/tmp.

ENV HOME=/tmp
WORKDIR /workspace
CMD ["ruby", "--version"]
