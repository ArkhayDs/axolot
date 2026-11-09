# POC Axolot - état et reste à faire

Démo PDG faite le 2026-11-04, retours dans « Prochains chantiers ». Dépôt privé `ArkhayDs/axolot`, rien de poussé sans demande explicite.

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

## UI (`/impeccable`, français, peu de texte, icônes animées)
- **Résultats** : base cumulée, synthèse pour non-tech en tête, gagnants qualité / vitesse / compromis par thème.
- **Atlas** : graphe dense (canvas) machine, modèles, thèmes, scénarios, questions, documents sources et Benchmarks sur `GET /api/map` et `GET /api/cases`, classements, et direct piloté par `GET /api/events` : impulsions sur le vrai chemin de la question, point qui change à la réponse notée. Sélection d'un modèle : chaque question prend son résultat. Sélection au clic sur les nœuds, barre d'état en bas de page avec recherche et Recentrer.
- **Une page par cas** : choix des modèles, résultats côte à côte par item, vérifications, sortie, pour le RAG les passages retrouvés et la source attendue ouvrable (citation surlignée).
- **Résultats** : tableaux et graphiques, charger le skill `dataviz`.
- **Réglages** : machine détectée, bibliothèque de modèles avec aptitude (ok / juste / trop gros) et téléchargement avec progression, connexions, corpus et paramètres RAG, envoi de fichiers, indexation.
- **Style** : sombre, liquid glass limité à la navigation et aux panneaux flottants, jamais sur les tableaux ni les chiffres. Axolotl gravé, vue de dessus, comme icône (`web/public/logo.png`, `favicon.png`). `prefers-reduced-motion` respecté. CSS selon `css-conventions.md`.

## API
`GET /api/status` | `GET /api/events` (SSE : `item_start`, `delta`, `item_end`, `campaign_end`, `pull`, `ingest`) | `GET,PUT /api/settings` | `GET,POST /api/providers`, `DELETE /api/providers/{id}` | `GET /api/models`, `POST /api/models/pull` | `GET /api/cases`, `PUT /api/cases/{id}/models` | `GET,POST /api/corpus`, `GET /api/corpus/{name}/files/{file}`, `POST /api/corpus/{name}/files`, `POST /api/corpus/ingest` | `GET,POST /api/campaigns`, `GET /api/campaigns/{id}`, `POST /api/campaigns/{id}/cancel`.

## Modèles de la démo
`gemma4:e4b-it-q4_K_M` (6,6 Go), `qwen3.6:27b-q4_K_M` (17 Go), `bge-m3` (1,2 Go, embeddings). `qwen3.6:35b-a3b` (23 Go) est juste en RAM avec Colima.

## Prochains chantiers (retours de la démo, recherches à refaire en les attaquant)
Axolot est un outil réel de comparaison, la démo n'était qu'un point d'étape.

### 1. Modèles frontier et cloud
Point de comparaison principal : Claude sur abonnement, seul accès disponible (aucune clé API). Plan validé le 2026-11-05.
- PR 1 : pont local exposant `claude -p` comme une API compatible OpenAI, ajouté comme simple connexion. Loopback uniquement : faire passer d'autres utilisateurs par l'abonnement est interdit, le futur login d'Axolot devra passer par des clés API.
- PR 2 : correctifs de la connexion par clé API, qui échoue aujourd'hui avec Claude (`temperature`, schéma sans `strict`). Testés sur serveur simulé, faute de clé.
- PR 1 mesurée le 2026-11-09 (Max 20x), un Benchmark complet par modèle :

| Modèle | Score (réponses mesurées) | Erreurs | Durée | 1er token p50 | Total p50 | Débit p50 | Tokens entrée / sortie |
|---|---|---|---|---|---|---|---|
| `claude-haiku-5-5` | 97,4 | 0 / 66 | 4 min 57 | 2,9 s | 3,9 s | 298 t/s | 113 504 / 28 385 |
| `claude-opus-5-5` | 99,2 | 1 / 66 | 19 min 29 | 1,8 s | 3,4 s | 116 t/s | 110 441 / 19 631 |

- Quota : toujours 2 % après les deux Benchmarks (~270 000 tokens). Compteur affiché et délai de mise à jour non vérifiés, à recontrôler au prochain.
- Erreur Opus : « retried its structured output » sur `routage-perimetres`, la CLI a rejeté son premier appel `StructuredOutput`. Le pont note désormais la première tentative, comme pour un modèle local ; une réponse en texte libre est notée aussi.
- Une réponse en erreur (pont, quota, délai) est « non mesurée » : hors moyenne, sans note 0.
- Comparabilité :
  - Notes comparables aux modèles locaux.
  - Le cloud tourne à sa température par défaut et avec raisonnement, le local à 0,3 sans raisonnement.
  - Temps = attente réelle : ~1 s de lancement de Claude Code, 1er token retardé par la réflexion du modèle (cas à schéma).
  - ~600 tokens d'entrée cachés par question (consigne de Claude Code, même avec `--system-prompt ""`), absents d'un appel par clé API.
  - Débit possiblement surestimé si les tokens de réflexion sont comptés en sortie (non vérifié) : Haiku jusqu'à 877 t/s.
  - Note sous le tableau de Résultats dès qu'un modèle `claude-*` apparaît : les runs ne stockent que le nom de la connexion, pas son type (PR 4).
- Corpus réels envoyés hors machine : signalé dans le panneau Connexions, seul le corpus fictif part sans réserve.
- Plus tard : pont Codex sur la machine du PDG, avec son compte (GPT Astra, abonnement uniquement). Mesure « Astra via Codex », éligibilité à un usage scripté à vérifier.

### 2. Modèles locaux spécialisés
- Tester des modèles supplémentaires, dont un modèle spécialisé français (Chocolatine, nom et base à vérifier).
- Spécialités déclarées dans le catalogue (français, anglais, juridique, etc.), à confronter aux scores : une spécialité déclarée reste une hypothèse tant qu'un thème ne la mesure pas.
- Affiner le scoring. Thèmes à ajouter pour que ça se mesure : cas en anglais et cas juridiques, absents aujourd'hui.
- Routage automatique vers plusieurs modèles pour le RAG d'entreprise : à évaluer dans Axolot avant de le monter dans `ai-local`.

### 3. Modèles décisionnels
- JEV (TypeSafe AI) : connecteur, compte ouvert mais solde à 0,00 $, chercher des crédits d'essai. Modèle de décision sans génération de texte, à comparer sur `routage-sentinelle`. Clarifier les données envoyées au fournisseur.
- Modèles locaux du même type, plus lents : nom exact de cette famille à retrouver.

## Ensuite
- Nom de machine avec le type d'appareil (MacBook Pro ou Mac mini, via `system_profiler`). Le libellé est la clé de la base cumulée : prévoir la migration des snapshots existants, sinon la machine se scinde en deux dans l'Atlas.
- Retester le hot reload sans `usePolling`, `mountInotify` étant actif dans Colima.
- Atlas : molette de souris : elle déplace maintenant la vue, le zoom passe par le pincement ou Ctrl.
- Énergie par modèle : script hôte relevant `ioreg` pendant les Benchmarks (sans `sudo`, sur batterie, toute la machine ; `powermetrics` pour CPU/GPU), croisé avec les réponses, en Wh par réponse.
- Thème de Benchmark sur l'appel d'outils, avec date et heure comme premier outil.
- Plusieurs machines : export/import des résultats, le filtre machine n'a tourné qu'avec une seule.
- Login, puis exposition au-delà de loopback.
- Atlas : chevauchements de libellés restants, place des Benchmarks au centre.
- Formats réels : PDF multi-colonnes, exports Outlook, mails HTML à éprouver.
- Intervalles de confiance sur les Benchmarks répétés, à faire de la main de Raphaël.
- Grand format carré du logo si besoin : vue de dessus gravée `~/Downloads/axolot-logos/Gemini_Generated_Image_2qwabh2qwabh2qwa.jpg` (hors dépôt).
- Revoir la DA et les noms de menus pour les rendre cohérents avec Axolot et l'axolotl, en restant labo IA. Badges du README à reconsidérer.
- `DESIGN.md` à générer depuis l'interface finale.
- Pause explicative du RAG, à la demande de Raphaël : il doit en mettre un en place pour l'entreprise (`~/Gitlab/projects/ai-local`). Le parcours tuteur de `ai-local/notes-labo-rag.md` reste à faire de sa main.
- Recommandations à partir des résultats mesurés, partage entre machines par export/import ou synchronisation via VPS ou m710q, app desktop.
- Notation du code par exécution dans un bac à sable.
