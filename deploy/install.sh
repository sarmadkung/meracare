#!/usr/bin/env bash
#
# Prepare an Ubuntu server to run the GenxCare API, and install the units that
# keep it running and up to date. Run as root on the droplet:
#
#   /opt/genxcare/src/deploy/install.sh
#
# Safe to re-run: it installs the current copy of every unit and restarts
# nothing that is already correct. It does not write /etc/genxcare/api.env —
# secrets are placed by hand, once, and never live in this repository.

set -euo pipefail

REPO_URL=https://github.com/sarmadkung/meracare.git
REPO_DIR=/opt/genxcare/src
ENV_FILE=/etc/genxcare/api.env
GO_VERSION=1.24.5

# Overridable only to rehearse an install from a branch before it is merged.
# The deploy timer still follows main unless told otherwise.
BRANCH="${GENXCARE_DEPLOY_BRANCH:-main}"

[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }

echo "==> packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq git curl ufw debian-keyring debian-archive-keyring apt-transport-https

echo "==> swap"
if ! swapon --show | grep -q /swapfile; then
	fallocate -l 1G /swapfile
	chmod 600 /swapfile
	mkswap -q /swapfile
	swapon /swapfile
	grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >>/etc/fstab
fi
echo 'vm.swappiness=10' >/etc/sysctl.d/99-swappiness.conf
sysctl -q --system

echo "==> firewall"
ufw allow OpenSSH >/dev/null
ufw allow 80/tcp >/dev/null
ufw allow 443/tcp >/dev/null
ufw --force enable >/dev/null

echo "==> go $GO_VERSION"
if ! /usr/local/go/bin/go version 2>/dev/null | grep -q "go$GO_VERSION "; then
	curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
	rm -rf /usr/local/go
	tar -C /usr/local -xzf /tmp/go.tgz
	rm -f /tmp/go.tgz
fi

echo "==> service user and directories"
id genxcare >/dev/null 2>&1 || useradd --system --create-home --home-dir /var/lib/genxcare \
	--shell /usr/sbin/nologin genxcare
install -d -o root -g root -m 0755 /opt/genxcare
install -d -o root -g root -m 0755 /opt/genxcare/bin /opt/genxcare/state
install -d -o root -g genxcare -m 0750 /etc/genxcare
install -d -o genxcare -g genxcare -m 0755 /var/lib/genxcare

echo "==> checkout"
if [[ ! -d $REPO_DIR/.git ]]; then
	git clone --quiet "$REPO_URL" "$REPO_DIR"
fi
git -C "$REPO_DIR" config remote.origin.fetch '+refs/heads/*:refs/remotes/origin/*'
git -C "$REPO_DIR" fetch --quiet origin "$BRANCH"
git -C "$REPO_DIR" reset --quiet --hard "origin/$BRANCH"

echo "==> caddy"
if ! command -v caddy >/dev/null; then
	curl -fsSL 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' |
		gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
	curl -fsSL 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
		-o /etc/apt/sources.list.d/caddy-stable.list
	apt-get update -qq
	apt-get install -y -qq caddy
fi
install -d -o caddy -g caddy -m 0755 /var/log/caddy
install -m 0644 "$REPO_DIR/deploy/Caddyfile" /etc/caddy/Caddyfile
systemctl reload caddy 2>/dev/null || systemctl restart caddy

echo "==> units"
install -m 0644 "$REPO_DIR/deploy/genxcare-api.service" /etc/systemd/system/
install -m 0644 "$REPO_DIR/deploy/genxcare-deploy.service" /etc/systemd/system/
install -m 0644 "$REPO_DIR/deploy/genxcare-deploy.timer" /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now genxcare-deploy.timer

if [[ ! -f $ENV_FILE ]]; then
	cat >&2 <<-EOF

		$ENV_FILE does not exist yet, so the API cannot start.
		Create it (owner root:genxcare, mode 640) from apps/api/.env.example with
		production values, then run: systemctl start genxcare-deploy
	EOF
	exit 0
fi

systemctl enable genxcare-api >/dev/null
echo "==> first deploy"
systemctl start genxcare-deploy
systemctl --no-pager --lines=0 status genxcare-api || true
