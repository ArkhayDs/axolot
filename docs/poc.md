# POC Axolot - état et reste à faire

Démo PDG : 2026-11-04 à 16h30. Dépôt privé `ArkhayDs/axolot`, rien de poussé sans demande explicite. Commits atomiques sur `main` sans validation individuelle jusqu'à la démo (accord de Raphaël le 2026-11-04).

## Objectif de la démo
Montrer quel modèle local convient à quelle tâche, à quelle vitesse et avec quelle qualité, RAG compris, sur la machine présente. Campagne lancée à l'avance + un cas en direct.

## Fait
- Backend Go complet et testé : connexions, téléchargements, cas, corpus, indexation, campagnes, événements SSE, analytics.
- Corpus fictif "Groupe Hélianthe" (`corpus/demo`) et 8 cas, 41 items (`cases/`).

## Premier essai réel (2026-11-04, API seule, sans front)
| Modèle | Score moyen | Temps médian | tok/s médian | Campagne (41 items) |
|---|---|---|---|---|
| `gemma4:e4b-it-q4_K_M` | 99 | 0,6 s | 169 | 55 s |
| `qwen3.6:27b-q4_K_M` | 100 | 4,3 s | 22 | 236 s |

- Connexion conteneur vers Ollama, indexation (28 extraits, 9 fichiers), campagne et notation fonctionnent de bout en bout.
- **Cas trop faciles** : les deux modèles frôlent 100 %, y compris sur les pièges. Les réponses relues sont correctes, mais ce score ne départage pas les modèles. Le prompt RAG aide aussi beaucoup : il demande explicitement la version la plus récente et de vérifier l'entité.
- Le seul échec (Gemma, `python-top-stores`, vérification regex) reste à relire : défaut du modèle ou vérification trop stricte.
- Ces deux essais restent en base (noms `Essai ...`), à supprimer avant la démo.

## Reste à faire, dans l'ordre
0. Dès le début de la reprise, lancer un agent en arrière-plan pour ajouter des cas durcis, validés par Raphaël : calculs à partir de plusieurs règles, croisement de documents, questions ambiguës, RAG sans consigne qui annonce les pièges, documents plus longs. Objectif : séparer réellement les deux modèles. Il écrit dans `cases/` et `corpus/demo/` uniquement, sans changer les cas existants, puis vérifie avec `make test`.
1. Front Svelte 5 dans `web/` (`package.json`, `package-lock.json`, `vite.config`, scripts `dev`, `build`, `check`). Le `Dockerfile` et `compose.yaml` l'attendent déjà. Proxy Vite `/api` vers `process.env.AXOLOT_API`, avec `web/embed.go` et `web/dist/.gitkeep` à conserver.
2. Vérifier avec Ollama lancé : `host.docker.internal` depuis Colima, modèles, indexation, une campagne réelle.
3. Lancer la campagne de démo avant 16h30, vérifier les scores et les pièges (faux positifs possibles).
4. README court, captures desktop + mobile via Playwright.

## UI (`/impeccable`, français, peu de texte, icônes animées)
- **Direction** : grande synthèse pour non-tech, gagnants qualité / vitesse / compromis par tâche.
- **Labo en direct** : réseau de neurones animé (canvas) piloté par `GET /api/events` : modèles reliés aux cas, impulsions sur les tokens, score en fin d'item, sortie en direct.
- **Une page par cas** : choix des modèles, résultats côte à côte par item, vérifications, sortie, pour le RAG les passages retrouvés et la source attendue ouvrable (citation surlignée).
- **Résultats** : tableaux et graphiques, charger le skill `dataviz`.
- **Réglages** : machine détectée, bibliothèque de modèles avec aptitude (ok / juste / trop gros) et téléchargement avec progression, connexions, corpus et paramètres RAG, envoi de fichiers, indexation.
- **Style** : sombre, liquid glass limité à la navigation et aux panneaux flottants, jamais sur les tableaux ni les chiffres. Axolotl en SVG original comme icône. `prefers-reduced-motion` respecté. CSS selon `css-conventions.md`.

## API
`GET /api/status` | `GET /api/events` (SSE : `item_start`, `delta`, `item_end`, `campaign_end`, `pull`, `ingest`) | `GET,PUT /api/settings` | `GET,POST /api/providers`, `DELETE /api/providers/{id}` | `GET /api/models`, `POST /api/models/pull` | `GET /api/cases`, `PUT /api/cases/{id}/models` | `GET,POST /api/corpus`, `GET /api/corpus/{name}/files/{file}`, `POST /api/corpus/{name}/files`, `POST /api/corpus/ingest` | `GET,POST /api/campaigns`, `GET /api/campaigns/{id}`, `POST /api/campaigns/{id}/cancel`.

## Modèles de la démo
`gemma4:e4b-it-q4_K_M` (6,6 Go), `qwen3.6:27b-q4_K_M` (17 Go), `bge-m3` (1,2 Go, embeddings). `qwen3.6:35b-a3b` (23 Go) est juste en RAM avec Colima.

## Après la démo
- Pause explicative du RAG, à la demande de Raphaël : il doit en mettre un en place pour l'entreprise (`~/Gitlab/projects/ai-local`). Le parcours tuteur de `ai-local/notes-labo-rag.md` reste à faire de sa main.
- JEV (TypeSafe AI) : connecteur si le compte est ouvert. Modèle de décision sans génération de texte, à comparer sur `routage-sentinelle`. Clarifier les données envoyées au fournisseur.
- Référence frontier sans clé API (avant la démo si le temps le permet, après le front et une campagne locale réussie) : script hôte exposant `claude -p` et l'abonnement Max comme une API compatible OpenAI, ajouté comme simple connexion. À vérifier : sortie structurée, mesures de temps, conditions de l'abonnement. Qualité comparable, vitesse moins directe (réseau, surcoût de Claude Code). Présenter comme référence cloud, hors démo 100 % locale.
- Recommandations à partir des résultats mesurés, partage entre machines par export/import ou synchronisation via VPS ou m710q, app desktop.
- Notation du code par exécution dans un bac à sable.
