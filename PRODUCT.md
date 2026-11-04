# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users
- Présentateur : Raphaël, développeur solo, lit les chiffres et pilote l'outil.
- Public de la démo : le PDG, non technique, à côté du présentateur devant le Mac. Peut suivre tout le parcours ou s'arrêter aux résultats.

## Product Purpose
Comparer des modèles d'IA sur des cas à résultat attendu. Réponse attendue : quel modèle convient à quelle tâche, à quelle vitesse et avec quelle qualité, sur cette machine. Succès : une décision lisible et vérifiable, sans connaissance technique.

## Positioning
La notation est déterministe : l'outil ne dépend d'aucune IA autre que les modèles testés. Chaque score remonte à des vérifications et, pour le RAG, à une citation que le présentateur ouvre à la main.

## Operating Context
Démonstration en intérieur sur l'écran d'un Mac, un Benchmark lancé à l'avance plus un scénario en direct. Parcours prévu : présentation de l'outil, résultats, direct, preuve. Le public peut écourter à tout moment, donc chaque étape tient seule.

## Capabilities and Constraints
- Un Benchmark et une question à la fois : le parallélisme fausserait les temps.
- Le code généré n'est jamais exécuté. Il est noté par vérifications statiques.
- Aucune authentification : usage en loopback uniquement.
- Les chiffres viennent de l'API : ne rien inventer ni arrondir au point de changer un classement.

## Brand Commitments
- Nom : Axolot. Icône : axolotl en SVG original.
- Interface en français, peu de texte, icônes animées.
- Sombre. Liquid glass limité à la navigation et aux panneaux flottants, jamais sur les tableaux ni les chiffres.
- Atlas interactif des modèles, machines, thèmes, scénarios, questions et documents en page principale, animé seulement par les événements réels.

## Evidence on Hand
- Benchmarks réels en base, cumulés, corpus fictif "Groupe Hélianthe" dans `corpus/demo`, scénarios dans `cases/`.
- Aucun client, témoignage ni benchmark externe : ne pas en inventer.

## Product Principles
- La synthèse avant le détail, et le détail toujours accessible.
- Chaque résultat se vérifie : score, vérifications, sortie, source.
- Les états réels se voient : modèle hors ligne, échec, Benchmark interrompu.
- Le mouvement montre ce qui se passe réellement, jamais une activité simulée présentée comme réelle.

## Accessibility & Inclusion
`prefers-reduced-motion` respecté. Couleur jamais seule porteuse de sens. Navigation clavier complète.
