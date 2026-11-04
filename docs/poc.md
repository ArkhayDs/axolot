# POC Axolot - état et reste à faire

Démo PDG : 2026-11-04 à 16h30. Dépôt privé `ArkhayDs/axolot`, rien de poussé sans demande explicite. Commits atomiques sur `main` sans validation individuelle jusqu'à la démo (accord de Raphaël le 2026-11-04).

## Objectif de la démo
Montrer quel modèle local convient à quelle tâche, à quelle vitesse et avec quelle qualité, RAG compris, sur la machine présente. Campagne lancée à l'avance + un cas en direct.

## Fait
- Backend Go complet et testé : connexions, téléchargements, cas, corpus, indexation, campagnes, événements SSE, analytics.
- Corpus fictif "Groupe Hélianthe" (`corpus/demo`, 16 fichiers, 57 extraits) et 13 cas, 66 items (`cases/`), dont 5 cas durcis.
- Front Svelte 5 embarqué dans le binaire : Atlas, Benchmarks, Scénarios, Résultats, puis Chat et Réglages. README et captures dans `docs/captures`.

## Base cumulée (2026-11-04, 2 Benchmarks complets, température 0,3)
| Modèle | Score cumulé | Réponses | Temps médian du dernier passage |
|---|---|---|---|
| `gemma4:e4b-it-q4_K_M` | 85,5 | 138 | 0,6 s |
| `qwen3.6:27b-q4_K_M` | 94,3 | 132 | 5,9 s |

- Les réponses antérieures à l'empreinte de question restent en base, hors stats. Rien n'est supprimé.
- Échecs relus sur les 2 passages : aucun faux positif trouvé. Valeurs attendues des 6 calculs revérifiées à la main. Les scores bougent d'un passage à l'autre : c'est voulu, une seule mesure ne prouve pas un petit écart.
- `python-top-stores` (Gemma) : code correct, signature non respectée alors que l'énoncé la demandait exacte. Défaut du modèle, vérification conservée.
- `lecture-km-brico-seule` : 0/100 pour 105,8 au lieu de 105,84 sur le dernier passage (égalité stricte), conservé.
- Le prompt RAG de `internal/rag/rag.go` annonce toujours les pièges, seuls les cas `lecture-documents` y échappent.
- Benchmarks : 8 min 54 sur batterie, 11 min 58 sur secteur, avec l'interface ouverte et en cours d'usage sur le second. Ne permet pas de conclure à un effet de l'alimentation.
- Gel d'onglet vu une fois sur l'Atlas : cause non trouvée, non reproduit, protections ajoutées. Garder un onglet de secours sur Résultats.

## Reste à faire
- Répéter le parcours sur `http://localhost:5090` : Atlas, un scénario en direct, Résultats, citation ouverte, Chat avec et sans Documents.

## UI (`/impeccable`, français, peu de texte, icônes animées)
- **Résultats** : base cumulée, synthèse pour non-tech en tête, gagnants qualité / vitesse / compromis par thème.
- **Atlas** : graphe dense (canvas) machine, modèles, thèmes, scénarios, questions, documents sources et Benchmarks sur `GET /api/map` et `GET /api/cases`, classements, et direct piloté par `GET /api/events` : impulsions sur le vrai chemin de la question, point qui change à la réponse notée. Sélection d'un modèle : chaque question prend son résultat. Sélection au clic sur les nœuds, barre d'état en bas de page avec recherche et Recentrer.
- **Une page par cas** : choix des modèles, résultats côte à côte par item, vérifications, sortie, pour le RAG les passages retrouvés et la source attendue ouvrable (citation surlignée).
- **Résultats** : tableaux et graphiques, charger le skill `dataviz`.
- **Réglages** : machine détectée, bibliothèque de modèles avec aptitude (ok / juste / trop gros) et téléchargement avec progression, connexions, corpus et paramètres RAG, envoi de fichiers, indexation.
- **Style** : sombre, liquid glass limité à la navigation et aux panneaux flottants, jamais sur les tableaux ni les chiffres. Axolotl en SVG original comme icône. `prefers-reduced-motion` respecté. CSS selon `css-conventions.md`.

## API
`GET /api/status` | `GET /api/events` (SSE : `item_start`, `delta`, `item_end`, `campaign_end`, `pull`, `ingest`) | `GET,PUT /api/settings` | `GET,POST /api/providers`, `DELETE /api/providers/{id}` | `GET /api/models`, `POST /api/models/pull` | `GET /api/cases`, `PUT /api/cases/{id}/models` | `GET,POST /api/corpus`, `GET /api/corpus/{name}/files/{file}`, `POST /api/corpus/{name}/files`, `POST /api/corpus/ingest` | `GET,POST /api/campaigns`, `GET /api/campaigns/{id}`, `POST /api/campaigns/{id}/cancel`.

## Modèles de la démo
`gemma4:e4b-it-q4_K_M` (6,6 Go), `qwen3.6:27b-q4_K_M` (17 Go), `bge-m3` (1,2 Go, embeddings). `qwen3.6:35b-a3b` (23 Go) est juste en RAM avec Colima.

## Après la démo
- Énergie par modèle : script hôte relevant `ioreg` pendant les Benchmarks (sans `sudo`, sur batterie, toute la machine ; `powermetrics` pour CPU/GPU), croisé avec les réponses, en Wh par réponse.
- Thème de Benchmark sur l'appel d'outils, avec date et heure comme premier outil.
- Plusieurs machines : export/import des résultats, le filtre machine n'a tourné qu'avec une seule.
- Login, puis exposition au-delà de loopback.
- Atlas : chevauchements de libellés restants, place des Benchmarks au centre.
- Formats réels : PDF multi-colonnes, exports Outlook, mails HTML à éprouver.
- Intervalles de confiance sur les Benchmarks répétés, à faire de la main de Raphaël.
- `DESIGN.md` à générer depuis l'interface finale.
- Pause explicative du RAG, à la demande de Raphaël : il doit en mettre un en place pour l'entreprise (`~/Gitlab/projects/ai-local`). Le parcours tuteur de `ai-local/notes-labo-rag.md` reste à faire de sa main.
- JEV (TypeSafe AI) : connecteur si le compte est ouvert. Modèle de décision sans génération de texte, à comparer sur `routage-sentinelle`. Clarifier les données envoyées au fournisseur.
- Référence frontier sans clé API (avant la démo si le temps le permet, après le front et une campagne locale réussie) : script hôte exposant `claude -p` et l'abonnement Max comme une API compatible OpenAI, ajouté comme simple connexion. À vérifier : sortie structurée, mesures de temps, conditions de l'abonnement. Qualité comparable, vitesse moins directe (réseau, surcoût de Claude Code). Présenter comme référence cloud, hors démo 100 % locale.
- Recommandations à partir des résultats mesurés, partage entre machines par export/import ou synchronisation via VPS ou m710q, app desktop.
- Notation du code par exécution dans un bac à sable.
