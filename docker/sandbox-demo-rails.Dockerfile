FROM ovx-sandbox-ruby:latest
WORKDIR /opt/ovx-demo
COPY examples/demo-rails/Gemfile examples/demo-rails/Gemfile.lock ./
RUN bundle config set --global frozen true && bundle install
ENV HOME=/tmp RAILS_ENV=test
WORKDIR /workspace
CMD ["bundle", "exec", "rspec"]
