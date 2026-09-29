ARG RUBY_VERSION=3.4.2
FROM ruby:${RUBY_VERSION}-slim
RUN apt-get update && apt-get install -y --no-install-recommends build-essential libpq-dev postgresql libvips git ca-certificates && rm -rf /var/lib/apt/lists/*
ARG LOCAL_UID=501
ARG LOCAL_GID=20
RUN groupadd -o -g ${LOCAL_GID} ovx && useradd -o -u ${LOCAL_UID} -g ovx -m ovx
WORKDIR /opt/project
COPY Gemfile Gemfile.lock ./
RUN bundle config set --global frozen true && bundle install
COPY entrypoint.sh /usr/local/bin/ovx-rails-tests
RUN chmod 755 /usr/local/bin/ovx-rails-tests
ENV RAILS_ENV=test RACK_ENV=test HOME=/tmp
WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/ovx-rails-tests"]
