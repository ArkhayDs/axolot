# 002 - Données locales, synchro via axolhost, app desktop

**Date:** 2026-11-23
**Statut:** Validé

## Décision

Les données d'Axolot restent sur la machine par défaut. Un export et un import manuels sont toujours disponibles. La synchro entre machines est optionnelle : elle passe par axolhost, un second binaire du même dépôt (`cmd/axolhost`) que chaque utilisateur héberge lui-même, avec une interface d'admin pour gérer ses machines. L'export et la synchro partagent un seul format, avec un numéro de version. L'app desktop est une coquille Tauri qui lance le binaire Go en sidecar, sur une base SQLite embarquée avec sqlite-vec à la place de PostgreSQL et pgvector.

## Pourquoi

- Les modèles locaux se testent hors ligne : un serveur ne doit jamais être obligatoire. Hors ligne, seuls les scénarios qui ont besoin d'internet sont perdus.
- Retrouver les benchmarks d'une machine sur une autre permet, à terme, un réseau de machines testées.
- Les résultats d'une campagne terminée ne changent plus : la synchro ne fait qu'ajouter, sans conflit à résoudre. Les campagnes et les résultats ont déjà des identifiants UUID.
- Un seul format pour l'export et la synchro : une seule logique à écrire et à tester.
- Une app desktop ne peut pas embarquer PostgreSQL simplement. Sans base embarquée, elle ne serait qu'une fenêtre sur `localhost` qui exige Docker.
- La stack (front Svelte, logique Go, Tauri) est celle de toutes les apps perso : document « Stack des apps perso » dans Linear (projet Harnais perso).

## Alternatives écartées

- **Serveur central obligatoire** : casse l'usage hors ligne.
- **Simple saisie de l'adresse du serveur** : n'importe qui connaissant l'adresse pourrait lire ou écraser des benchmarks.
- **Un serveur pour plusieurs utilisateurs** : inutile tant qu'Axolot n'a qu'un utilisateur. Chacun héberge son axolhost.
- **Interface d'axolhost qui reprend les écrans d'Axolot** : les résultats de toutes les machines s'affichent déjà dans Axolot après la synchro.
- **Garder PostgreSQL + Docker pour l'app desktop** : l'app exigerait Docker.

## Conséquences

- **Sécurité de la synchro** : appairage par code à usage unique et de courte durée, puis un jeton par machine, haché sur axolhost, révocable. HTTPS obligatoire. Le homelab personnel reste derrière Tailscale.
- **Données jamais synchronisées** : les clés API, chiffrées avec l'`AXOLOT_SECRET` de chaque machine et illisibles ailleurs.
- **Option « chiffres seuls »** : sans les réponses des modèles ni les passages du corpus. Le corpus RAG peut contenir des documents internes : à utiliser pour les machines pro.
- **Machine identifiée sur chaque campagne** : identifiant stable et nom lisible.
- **Format versionné** : axolhost accepte les anciennes versions prises en charge et refuse les autres en entier, jamais d'enregistrement partiel.
- **Nouveaux utilisateurs** : chacun part de zéro. Un jeu de résultats de départ, pour consulter sans tout refaire, reste à concevoir.
- **Migration vers SQLite** : gros chantier, à découper en plusieurs PR.
- **Suivi** : projet Linear « Axolot - Données portables et app desktop ».
