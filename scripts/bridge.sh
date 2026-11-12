#!/bin/sh
# Builds the Claude bridge for this machine in the go container, then runs it on the host,
# where the keychain holds the Claude login. Adds its token and port to an existing .env, nothing else.
set -eu

if [ ! -f .env ]; then
	echo "no .env, run make setup first" >&2
	exit 1
fi
[ -z "$(tail -c 1 .env)" ] || echo >> .env
grep -q '^AXOLOT_BRIDGE_TOKEN=' .env || echo "AXOLOT_BRIDGE_TOKEN=$(openssl rand -hex 32)" >> .env
grep -q '^AXOLOT_BRIDGE_PORT=' .env || echo "AXOLOT_BRIDGE_PORT=5092" >> .env

docker compose run --rm \
	-e GOOS="$(uname -s | tr '[:upper:]' '[:lower:]')" \
	-e GOARCH="$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')" \
	go go build -o /src/bin/axolot-bridge ./cmd/axolot-bridge

# Only the bridge's own variables: it hands its environment to the CLI, which has no use for the app secrets.
token=$(sed -n 's/^AXOLOT_BRIDGE_TOKEN=//p' .env)
port=$(sed -n 's/^AXOLOT_BRIDGE_PORT=//p' .env)
echo "Connection: Réglages, Connexions, Connecter un compte Claude (after make up if the token was just added)"
AXOLOT_BRIDGE_TOKEN=$token AXOLOT_BRIDGE_PORT=$port exec bin/axolot-bridge
