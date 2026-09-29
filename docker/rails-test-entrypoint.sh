#!/bin/sh
set -eu
# A new database lives only in this container's tmpfs. No host DB is accessed.
PG_BIN=$(dirname "$(find /usr/lib/postgresql -name initdb -type f | head -1)")
export PATH="$PG_BIN:$PATH"
initdb -D /tmp/ovx-pg -A trust -U ovx > /tmp/ovx-initdb.log
pg_ctl -D /tmp/ovx-pg -l /tmp/ovx-postgres.log -o '-k /tmp -h 127.0.0.1' -w start > /dev/null
createdb -h /tmp -U ovx ovx_project_test
export DATABASE_URL=postgres://ovx@127.0.0.1/ovx_project_test
export SECRET_KEY_BASE=ovx-isolated-test-only-not-a-production-secret
bundle exec rails db:schema:load > /tmp/ovx-schema.log 2>&1 || { cat /tmp/ovx-schema.log; exit 1; }
exec "$@"
