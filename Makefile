.PHONY: help setup up down logs hardware test lint web-check dev

help: ## List targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*## /: /'

setup: ## Create .env with generated secrets, once
	@sh scripts/setup-env.sh

hardware: ## Detect this machine (a container only sees its VM)
	@sh scripts/detect-hardware.sh > data/hardware.json

up: hardware ## Build and start the app
	docker compose up -d --build db app

down: ## Stop the stack, data kept
	docker compose down

logs: ## Follow app logs
	docker compose logs -f app

dev: ## Start the Vite dev server with hot reload
	docker compose --profile dev up web

test: ## Run Go tests, database included
	docker compose up -d --wait db
	docker compose run --rm go go test ./...

lint: ## Format check and vet
	docker compose run --rm go sh -c 'test -z "$$(gofmt -l .)" && go vet ./...'

web-check: ## Type-check the front-end
	docker compose --profile dev run --rm web sh -c 'npm install && npm run check && npm test'
