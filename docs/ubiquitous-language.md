# Glossaire Axolot

Vocabulaire commun du code, des docs et des échanges. Un mot présent ici a exactement ce sens dans le projet. Termes en français tels qu'affichés dans l'interface, équivalent du code en `Translations`.

## Pages

### Atlas

**Definition**: page d'accueil, graphe machine, modèles, thèmes, scénarios, questions et documents reliés ; classement, lancement et direct d'un Benchmark.
**Translations**: map (code).
**Examples**: `web/src/pages/Map.svelte`, `web/src/components/map/`, route `/`.

### Chat

**Definition**: page pour poser une question libre aux modèles cochés et comparer leurs réponses, avec corpus, document ou image joints et outils des serveurs MCP en option.
**Examples**: `web/src/pages/Chat.svelte`.
**Not to be confused with**: Benchmark, seul à noter les réponses.

### Scénarios

**Definition**: liste des scénarios, puis une page par scénario avec les réponses des modèles côte à côte par question, leurs vérifications et passages RAG.
**Translations**: cases (code).
**Examples**: `web/src/pages/CaseList.svelte`, `CaseDetail.svelte`, route `/cas`.

### Résultats

**Definition**: cumul de toute la base par thème et par modèle (« Quel modèle pour quelle tâche ? ») : score, temps médian, verdicts. Pas de détail réponse par réponse.
**Translations**: results (code).
**Examples**: `web/src/pages/Results.svelte`, `EvalMatrix.svelte`, `StatsTable.svelte`.
**Not to be confused with**: Benchmarks, où se trouve le détail d'une exécution.

### Benchmarks

**Definition**: historique des Benchmarks ; le détail d'un Benchmark a les onglets Résumé, Temps, Réponses, Qualimétrie.
**Translations**: campaigns (code).
**Examples**: `web/src/pages/Benchmarks.svelte`, `BenchmarkDetail.svelte`.
**See also**: Grille des réponses.

### Réglages

**Definition**: machine détectée, connexions, bibliothèque de modèles, corpus et paramètres RAG, indexation.
**Translations**: settings (code).
**Examples**: `web/src/pages/Settings.svelte`.

## Exécution

### Benchmark

**Definition**: une exécution des scénarios choisis sur des modèles, une question et un modèle à la fois ; chacune alimente la base cumulée.
**Translations**: campaign (code, API et base).
**Examples**: `internal/runner/runner.go`, `store.CreateCampaign`, `benchmarkName()` dans `format.ts`.
**Not to be confused with**: « campagne », ancien nom affiché, encore présent dans les noms enregistrés.

### Scénario

**Definition**: cas d'usage à résultat attendu, rattaché à un thème, avec ses questions.
**Translations**: case (code).
**Examples**: `cases/*.json`, `cases.Case`.

### Question

**Definition**: une entrée d'un scénario, avec son résultat attendu et ses vérifications.
**Translations**: item (code).
**Examples**: `cases.Item`, `item_id`.

### Thème

**Definition**: famille de scénarios (Calcul, Code, Documents internes...) ; les Résultats sont ventilés par thème.
**Translations**: category (code).
**Examples**: `categoryLabel()` dans `format.ts`.

### Réponse

**Definition**: ce qu'un modèle a répondu à une question dans un Benchmark, avec sa note, ses temps et ses tokens.
**Translations**: run (code et base).
**Examples**: `store.Run`, table `runs`, `RunColumn.svelte`.

### Machine

**Definition**: matériel de l'hôte qui a exécuté un Benchmark, détecté avant le démarrage ; les résultats sont cumulés par machine.
**Translations**: hardware (code).
**Examples**: `scripts/detect-hardware.sh`, `data/hardware.json`, `internal/hardware`.

### Aptitude

**Definition**: verdict de la bibliothèque de modèles sur la taille d'un modèle face à la mémoire de la machine : OK, Juste, Trop gros, Inconnu.
**Translations**: fit (code).
**Examples**: `FitBadge.svelte`, `hardware.FitFor`.

### Connexion

**Definition**: serveur de modèles enregistré dans Réglages. Trois types : locale (Ollama), cloud par API (clé), cloud par abonnement (pont).
**Translations**: provider (code), `connection_type` `local` | `api` | `subscription`.
**Examples**: `ConnectionsPanel.svelte`, `internal/provider`.

### Pont

**Definition**: programme lancé sur l'hôte (`make bridge`) qui expose la CLI `claude` comme une API compatible OpenAI, pour utiliser un abonnement Claude.
**Translations**: bridge (code).
**Examples**: `cmd/axolot-bridge`, `ClaudeBridgeDialog.svelte`.

### Direct

**Definition**: suivi d'un Benchmark en cours sur l'Atlas : impulsions sur le chemin de la question, reste du graphe estompé, caméra qui peut suivre la question (Suivre), grille des réponses qui se remplit.
**Translations**: live (code).
**Examples**: `LivePanel.svelte`, `live.svelte.ts`, `GET /api/events`.

## Notation et mesure

### Score

**Definition**: note sur 100 d'une réponse, part des vérifications réussies, sans aucune IA ; 0 au-delà de 5 min.
**Examples**: `internal/scoring`, `scoring.Grade`.

### Vérification

**Definition**: contrôle déterministe d'une réponse (valeur attendue, citation, longueur, passage retrouvé...).
**Translations**: check (code).
**Examples**: `checkLabel()` dans `format.ts`, colonne `checks`.

### Réponse non mesurée

**Definition**: réponse en erreur (pont, quota, connexion) : hors moyenne, sans note 0.
**Translations**: error (code), `UNMEASURED_HINT`.
**Not to be confused with**: un score de 0, qui est une vraie mauvaise réponse.

### Empreinte de question

**Definition**: hash du contenu d'une question ; les réponses à une version antérieure restent en base mais sortent des chiffres.
**Translations**: item hash (code).
**Examples**: `item_hash`, `countedAnswer` dans `internal/store/map.go`.

### Base cumulée

**Definition**: ensemble des réponses comptées (questions à jour, hors Benchmark en cours), tous Benchmarks confondus, d'où sont tirés les Résultats et l'Atlas.
**Examples**: `cumulatedAnswers` dans `internal/store/map.go`.

### Verdicts

**Definition**: gagnants par thème : Qualité (meilleur score), Vitesse (plus rapide), Recommandé (le plus rapide à moins de 10 points du meilleur score).
**Translations**: quality, speed, compromise (code).
**Examples**: `analytics.Verdicts`, `EvalMatrix.svelte`.

### Temps net

**Definition**: temps d'un modèle Claude sans le lancement ni la fermeture de Claude Code, mesurés par le pont ; le temps brut est l'attente réelle.
**Translations**: net / gross (code), `NET_HINT`, `GROSS_HINT`.
**See also**: Pont.

### Qualimétrie

**Definition**: indicateurs de forme calculés sur les réponses (longueur, mots par phrase...), sans note globale ni IA.
**Translations**: qualimetry (code).
**Examples**: `web/src/lib/qualimetry.ts`, `QualityTable.svelte`.

### Grille des réponses

**Definition**: tableau question × modèle d'un Benchmark, une case par réponse ; dans le détail d'un Benchmark (onglet Réponses) et le direct de l'Atlas.
**Translations**: run matrix (code).
**Examples**: `RunMatrix.svelte`.
**Not to be confused with**: le tableau des Résultats, cumulé par thème.

### Température par défaut (T°)

**Definition**: la réponse a été produite à la température propre du modèle, qui a ignoré celle de la campagne (Claude par le pont, modèle de raisonnement relancé sans température).
**Translations**: `default_temperature` (code et base), `DEFAULT_TEMPERATURE_HINT`.
**Examples**: grille des réponses, détail d'une réponse dans Scénarios, classement de l'Atlas.

## RAG

### Corpus

**Definition**: dossier de documents servant de source au RAG ; seul le corpus fictif peut partir hors machine sans réserve.
**Examples**: `corpus/<dossier>`, `CorpusPanel.svelte`.

### Indexation

**Definition**: extraction, découpe en passages et embedding du corpus ; à relancer (Réglages, Indexer) après tout changement du corpus ou des paramètres RAG.
**Translations**: ingest (code).
**Examples**: `rag.Ingest`, `POST /api/corpus/ingest`.

### Passage

**Definition**: morceau de document retrouvé et transmis au modèle avec la question.
**Translations**: chunk, hit (code).
**Examples**: `ChatSources.svelte`, colonne `retrieved`.

### Piège

**Definition**: difficulté prévue d'une question RAG : réponse absente, version périmée, autre entité, ou sans piège.
**Translations**: trap (code).
**Examples**: `trapLabel()` dans `format.ts`.
