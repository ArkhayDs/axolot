# POC Axolot - état et reste à faire

Démo PDG : 2026-11-04 à 16h30. Dépôt privé `ArkhayDs/axolot`, rien de poussé sans demande explicite. Commits atomiques sur `main` sans validation individuelle jusqu'à la démo (accord de Raphaël le 2026-11-04).

## Objectif de la démo
Montrer quel modèle local convient à quelle tâche, à quelle vitesse et avec quelle qualité, RAG compris, sur la machine présente. Campagne lancée à l'avance + un cas en direct.

## Fait
- Backend Go complet et testé : connexions, téléchargements, cas, corpus, indexation, campagnes, événements SSE, analytics.
- Corpus fictif "Groupe Hélianthe" (`corpus/demo`, 16 fichiers, 57 extraits) et 13 cas, 66 items (`cases/`), dont 5 cas durcis.
- Front Svelte 5 embarqué dans le binaire : Direction, Labo, Cas, Résultats, Réglages. README et captures dans `docs/captures`.

## Campagne de démo (2026-11-04, 132 réponses, depuis l'interface)
| Modèle | Score moyen | Temps médian | tok/s médian |
|---|---|---|---|
| `gemma4:e4b-it-q4_K_M` | 86 | 0,6 s | 156 |
| `qwen3.6:27b-q4_K_M` | 95,5 | 5,2 s | 21 |

- Les cas durcis départagent : Questions ambiguës 50 / 83,3, Lecture de documents 25 / 100, Documents internes 96 / 100.
- Échecs relus un par un : aucun faux positif trouvé, valeurs attendues des 6 calculs revérifiées à la main. Calcul à 66,7 des deux côtés par coïncidence, sur des erreurs différentes.
- `python-top-stores` (Gemma) : code correct, signature non respectée alors que l'énoncé la demandait exacte. Défaut du modèle, vérification conservée.
- Limites connues : `lecture-km-brico-seule` donne 0/100 à Gemma pour 105,8 au lieu de 105,84 (égalité stricte). Le prompt RAG de `internal/rag/rag.go` annonce toujours les pièges, seuls les cas `lecture-documents` y échappent.

## Reste à faire
- Avant la démo, sur accord de Raphaël : supprimer de la base `Essai qwen...`, `Essai gemma...`, `Essai front` et `Campagne` (annulée, 0 réponse).
- Répéter le parcours sur `http://localhost:5090` : Direction, un cas en direct dans le Labo, citation ouverte dans une page de cas.

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
