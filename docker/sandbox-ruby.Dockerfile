FROM ruby:3.3-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates build-essential libpq-dev \
    && rm -rf /var/lib/apt/lists/*

# The demo uses Minitest; install it explicitly rather than relying on bundled gems.
RUN gem install minitest -v "~> 5.0" --no-document

ENV HOME=/tmp
WORKDIR /workspace
CMD ["ruby", "--version"]
