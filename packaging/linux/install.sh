#!/bin/sh
# Installs the Gateway as a hardened systemd service. Run as root after building
# the binary: go build -o mtandao-gateway ./cmd/gateway
set -eu
here="$(cd "$(dirname "$0")" && pwd)"
binary="${1:-./mtandao-gateway}"
[ -x "$binary" ] || { echo "binary not found: $binary" >&2; exit 1; }

id -u mtandao-gateway >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin mtandao-gateway
install -m 0755 "$binary" /usr/local/bin/mtandao-gateway
install -d -m 0750 -o root -g mtandao-gateway /etc/mtandao-gateway
[ -e /etc/mtandao-gateway/gateway.env ] || install -m 0640 -o root -g mtandao-gateway "$here/gateway.env.example" /etc/mtandao-gateway/gateway.env
install -m 0644 "$here/mtandao-gateway.service" /etc/systemd/system/mtandao-gateway.service
systemctl daemon-reload
echo "Installed. Next: enroll the Gateway, fill /etc/mtandao-gateway/router.json, then: systemctl enable --now mtandao-gateway"
