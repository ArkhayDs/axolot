# 001 - Distribution : code privé, installables publics

**Date:** 2026-11-23
**Statut:** Validé

## Décision

Le dépôt du code d'Axolot est privé. Les installables de l'app desktop (`.dmg`, `.exe`, paquet Linux) sont publiés dans un second dépôt public, `axolot-releases`, qui ne contient que des releases. À chaque tag de version, une GitHub Action du dépôt privé construit l'app sur macOS, Windows et Linux, puis dépose les fichiers dans une release du dépôt public. Le code n'a pas de licence.

## Pourquoi

- Les utilisateurs n'ont besoin que de l'installable, pas du code.
- Sur un dépôt public, GitHub ne permet ni d'interdire le clone, ni le téléchargement en ZIP, ni le fork. Seul un dépôt privé garde le code fermé.
- Les releases d'un dépôt privé ne sont téléchargeables que par les personnes qui ont accès au dépôt : il faut un second dépôt, public, pour les installables.
- Sans licence, personne n'a le droit de réutiliser le code, même pendant une période où il est visible. Une licence libre ne se retire pas sur une version déjà diffusée : elle se décide plus tard, en connaissance de cause.

## Alternatives écartées

- Dépôt de code public avec ses releases : le code devient copiable par tous.
- Dépôt de code privé avec ses releases : les installables restent inaccessibles hors des membres du dépôt.

## Conséquences

- L'Action a besoin d'un jeton d'accès en écriture au dépôt `axolot-releases`, enregistré dans les secrets du dépôt privé.
- Les builds du dépôt privé consomment le quota gratuit de GitHub Actions : une minute macOS compte pour 10, une minute Windows pour 2. Au-delà, les minutes sont facturées.
- Sans signature, macOS bloque l'app téléchargée au premier lancement (autorisation unique dans Réglages Système, Confidentialité et sécurité) et Windows affiche un avertissement SmartScreen. La signature macOS passe par le programme Apple Developer (99 USD par an).
- À mettre en place quand l'app desktop existe. D'ici là, le dépôt repasse en privé une fois sa période publique terminée.
