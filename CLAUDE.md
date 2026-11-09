# Axolot

## Project Config
- `branch_prefix`: AXO
- **Linear**: N/A pendant le POC (suivi dans `docs/poc.md`)

## Overview
Labo de comparaison de modèles IA (locaux, cloud, modèles de décision) sur des cas d'usage à résultat attendu. Chiffres pour le dev, vue de synthèse pour un public non technique. L'outil ne dépend d'aucune IA autre que les modèles testés : la notation est déterministe.

**Phase**: V1 fonctionnelle, outil réel en cours d'extension. État et reste à faire dans `docs/poc.md`.

| Couche | Stack |
|---|---|
| DB | PostgreSQL 17 + pgvector |
| Backend | Go 1.26, `net/http`, pgx, binaire unique avec le front embarqué |
| Frontend | Svelte 5 + Vite + TS, sans SvelteKit |
| Modèles | Ollama sur l'hôte (Metal hors conteneur), toute API compatible OpenAI |
| Infra | Docker Compose, écoute sur loopback uniquement |

Structure : `cmd/axolot` entrée, `cmd/axolot-bridge` pont Claude (hôte), `internal/{api,runner,rag,scoring,analytics,provider,store,hardware,catalog,secret}`, `cases/*.json` cas embarqués, `corpus/<dossier>` documents RAG, `web/` front, `scripts/` côté hôte.

## Commands
```bash
make setup      # crée .env et ses secrets, une seule fois
make up         # détecte le matériel, build et démarre (http://localhost:5090)
make dev        # Vite avec hot reload (http://localhost:5191), proxy vers app
make bridge     # pont Claude sur abonnement, sur l'hôte, au premier plan
make test       # tests Go, base comprise
make lint       # gofmt + go vet
make web-check  # svelte-check
```

## Gotchas
- **Matériel vu depuis l'hôte** : un conteneur ne voit que sa VM, `scripts/detect-hardware.sh` écrit `data/hardware.json` avant le démarrage.
- **Une campagne à la fois, un item à la fois** : le parallélisme fausserait les temps sur un seul GPU.
- **Citations du RAG** : `cases_test.go` vérifie que chaque `quote` existe mot pour mot dans le corpus, le présentateur les ouvre à la main.
- **Pas d'authentification** : ne pas exposer au-delà de loopback sans ajouter un login.
- **`AXOLOT_SECRET`** : le changer rend illisibles les clés API enregistrées.
- **Code généré jamais exécuté** : les cas de code sont notés par vérifications statiques.

## Pre-commit Sequence
`make lint` -> `make test` -> `make web-check` si le front change -> commit.
