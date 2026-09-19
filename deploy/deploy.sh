#!/usr/bin/env bash
#
# Deploy the GenxCare API when apps/api/VERSION changes on main.
#
# Run by genxcare-deploy.timer every two minutes. Ordinary commits change
# nothing: only a different value in apps/api/VERSION starts a deploy, so the
# release moment is an explicit edit in a pull request rather than a push.
#
# A deploy builds on this machine, applies migrations, restarts the service and
# waits for /readyz. If the new release does not become ready, the previous
# binary is put back and the deploy fails loudly in the journal — so a bad
# build costs a restart, not an outage.
#
#   journalctl -u genxcare-deploy -n 50    what the last run did
#   systemctl start genxcare-deploy        deploy now instead of waiting

set -euo pipefail

REPO_DIR=/opt/genxcare/src
BIN_DIR=/opt/genxcare/bin
STATE_DIR=/opt/genxcare/state
ENV_FILE=/etc/genxcare/api.env
LOCK_FILE=/var/lock/genxcare-deploy.lock
# Overridable only to rehearse a deploy from a branch before it is merged.
BRANCH="${GENXCARE_DEPLOY_BRANCH:-main}"
GO=/usr/local/go/bin/go
KEEP_RELEASES=3

# The checkout is rewritten mid-run, and bash reads a script as it executes it.
# Run from a private copy so an update cannot change the script under itself.
if [[ "${GENXCARE_DEPLOY_REEXEC:-}" != "1" ]]; then
	copy=$(mktemp /tmp/genxcare-deploy.XXXXXX.sh)
	cp "$0" "$copy"
	chmod +x "$copy"
	GENXCARE_DEPLOY_REEXEC=1 exec "$copy" "$@"
fi
trap 'rm -f "$0"' EXIT

log() { printf '%s\n' "$*"; }
fail() { printf 'deploy failed: %s\n' "$*" >&2; exit 1; }

# One deploy at a time. A run that overlaps the previous one simply steps aside;
# the timer will call again in two minutes.
exec 9>"$LOCK_FILE"
flock -n 9 || { log "another deploy is running; skipping this tick"; exit 0; }

[[ -f "$ENV_FILE" ]] || fail "$ENV_FILE is missing"
mkdir -p "$BIN_DIR" "$STATE_DIR"

git -C "$REPO_DIR" fetch --quiet origin "$BRANCH"

if ! version=$(git -C "$REPO_DIR" show "origin/$BRANCH:apps/api/VERSION" 2>/dev/null); then
	# The release mechanism is not on this branch yet. Nothing to deploy, and
	# nothing wrong: stay quiet rather than failing every two minutes.
	exit 0
fi
version=$(printf '%s' "$version" | tr -d '[:space:]')
[[ -n "$version" ]] || fail "apps/api/VERSION on origin/$BRANCH is empty"

deployed=$(cat "$STATE_DIR/version" 2>/dev/null || true)
if [[ "$version" == "$deployed" ]]; then
	exit 0
fi

commit=$(git -C "$REPO_DIR" rev-parse --short "origin/$BRANCH")
log "deploying version $version ($commit), replacing ${deployed:-nothing}"

git -C "$REPO_DIR" reset --quiet --hard "origin/$BRANCH"

api_bin="$BIN_DIR/api-$version-$commit"
migrate_bin="$BIN_DIR/migrate-$version-$commit"

# -p 1 builds one package at a time. This droplet has a single CPU and under a
# gigabyte of memory, and a parallel build is the one thing here that can
# exhaust it while the API is serving traffic.
build() {
	env -C "$REPO_DIR/apps/api" HOME=/var/lib/genxcare \
		"$GO" build -p 1 -trimpath \
		-ldflags "-s -w -X main.version=$version" \
		-o "$1" "$2"
}

log "building"
build "$api_bin" ./cmd/api || fail "go build ./cmd/api"
build "$migrate_bin" ./cmd/migrate || fail "go build ./cmd/migrate"

# Migrations run before the new binary starts, each in its own transaction under
# an advisory lock, so the schema is never behind the code that expects it.
log "applying migrations"
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a
"$migrate_bin" up || fail "migrations"

previous=""
if [[ -L "$BIN_DIR/api-current" ]]; then
	previous=$(readlink -f "$BIN_DIR/api-current")
fi

ln -sfn "$api_bin" "$BIN_DIR/api-current"
systemctl restart genxcare-api

# Ready means the process started, its configuration validated and the database
# answered. Anything less is not a deploy worth keeping.
port=$(sed -n 's/^PORT=//p' "$ENV_FILE" | head -1)
ready_url="http://127.0.0.1:${port:-8080}/readyz"

ready=false
for _ in $(seq 1 30); do
	if curl -fsS --max-time 2 "$ready_url" >/dev/null 2>&1; then
		ready=true
		break
	fi
	sleep 1
done

if [[ "$ready" != true ]]; then
	if [[ -n "$previous" && -x "$previous" ]]; then
		log "new release never became ready; rolling back to $(basename "$previous")"
		ln -sfn "$previous" "$BIN_DIR/api-current"
		systemctl restart genxcare-api
	else
		log "new release never became ready and there is nothing to roll back to"
	fi
	fail "version $version did not become ready at $ready_url"
fi

printf '%s\n' "$version" >"$STATE_DIR/version"
printf '%s\n' "$commit" >"$STATE_DIR/commit"
log "version $version is live"

# Keep a few past builds so a rollback has somewhere to go, and no more: each
# binary is about 18 MB.
ls -1t "$BIN_DIR"/api-* 2>/dev/null | grep -v '/api-current$' | tail -n +$((KEEP_RELEASES + 1)) | xargs -r rm -f
ls -1t "$BIN_DIR"/migrate-* 2>/dev/null | tail -n +$((KEEP_RELEASES + 1)) | xargs -r rm -f
