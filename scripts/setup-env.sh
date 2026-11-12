#!/bin/sh
# Creates .env from .env.example with random secrets. Never overwrites an existing .env,
# because changing AXOLOT_SECRET makes saved API keys unreadable.
set -eu

if [ -f .env ]; then
	echo ".env already exists, left untouched"
	exit 0
fi
sed \
	-e "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$(openssl rand -hex 24)/" \
	-e "s/^AXOLOT_SECRET=.*/AXOLOT_SECRET=$(openssl rand -hex 32)/" \
	-e "s/^AXOLOT_BRIDGE_TOKEN=.*/AXOLOT_BRIDGE_TOKEN=$(openssl rand -hex 32)/" \
	.env.example > .env
chmod 600 .env
echo ".env created"
