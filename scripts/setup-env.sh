#!/bin/sh
# Creates .env from .env.example with random secrets, or adds the keys an older .env lacks.
# Never overwrites a value, because changing AXOLOT_SECRET makes saved API keys unreadable.
set -eu

if [ ! -f .env ]; then
	sed \
		-e "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$(openssl rand -hex 24)/" \
		-e "s/^AXOLOT_SECRET=.*/AXOLOT_SECRET=$(openssl rand -hex 32)/" \
		-e "s/^AXOLOT_BRIDGE_TOKEN=.*/AXOLOT_BRIDGE_TOKEN=$(openssl rand -hex 32)/" \
		.env.example > .env
	chmod 600 .env
	echo ".env created"
	exit 0
fi
[ -z "$(tail -c 1 .env)" ] || echo >> .env
grep -q '^AXOLOT_BRIDGE_TOKEN=' .env || echo "AXOLOT_BRIDGE_TOKEN=$(openssl rand -hex 32)" >> .env
grep -q '^AXOLOT_BRIDGE_PORT=' .env || echo "AXOLOT_BRIDGE_PORT=5092" >> .env
