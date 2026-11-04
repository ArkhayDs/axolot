# Axolot : présentation

Adresse : http://localhost:5090. Mac branché. Second onglet ouvert sur Résultats, en secours.

## En une phrase
Axolot fait passer les mêmes épreuves à plusieurs IA sur notre propre machine, note chaque réponse automatiquement, et montre laquelle convient à quelle tâche.

## Pourquoi c'est intéressant
- **Local** : les modèles tournent sur ce Mac, sans envoyer les documents à l'extérieur.
- **Vérifiable** : la note vient de contrôles automatiques, pas d'une autre IA. Chaque score se rouvre jusqu'à la réponse et sa source.
- **Cumulatif** : chaque test enrichit la base, donc les chiffres s'affinent avec le temps, machine par machine.

## Le parcours, 5 minutes

**1. Atlas : la vue d'ensemble (1 min)**
- "Ici, tout ce qu'on a testé : la machine, les modèles au centre, et autour les thèmes, avec chaque question en petit point."
- Cliquer sur `gemma4` : "Vert, il réussit. Rouge, il échoue. Les thèmes où il est fort se rapprochent de lui."
- Cliquer de nouveau pour désélectionner, puis faire pareil avec `qwen3.6`.

**2. Résultats : la réponse à la question (1 min)**
- Montrer la matrice du haut : "Plus une case est rouge, plus le modèle a perdu de points."
- Le message : **Gemma répond environ 10 fois plus vite, Qwen est plus fiable sur le difficile. Sur le tri, le routage, le résumé et l'extraction, les deux font 100, donc le rapide suffit.**

**3. Scénarios : la preuve (1 min)**
- Ouvrir "Demandes à exécuter ou à faire préciser".
- Première question, "Annule ma réunion de jeudi avec Karim", alors que Sophie en a deux ce jour-là avec lui : "Gemma annule une réunion au hasard, Qwen demande laquelle. Sur une action qu'on ne peut pas annuler, ça change tout."

**4. Chat : l'IA sur nos documents (1 min)**
- Poser "Combien de jours de congés payés ai-je par an ?" sans option. Les modèles répondent de mémoire, ou pas du tout.
- Activer **Documents** et reposer la question : "Ils répondent à partir de nos documents et citent la source." C'est le RAG.
- Les documents sont fictifs : c'est le Groupe Hélianthe, inventé pour le test.

**5. En direct, si le temps le permet (1 min)**
- Atlas, **Lancer un Benchmark**, ne cocher que "Tri de tickets", lancer.
- "Chaque impulsion est un morceau de réponse reçu en vrai, rien n'est simulé."

**Si le temps manque :** faire seulement les étapes 2 et 4.

## Les mots à connaître
- **Benchmark** : une série d'épreuves passées par les modèles choisis.
- **Scénario** : un groupe de questions sur un même sujet, dont on connaît la bonne réponse.
- **Score** : le pourcentage de contrôles réussis, sur 100.
- **Temps médian** : la moitié des réponses arrive plus vite que ce temps.
- **RAG** : l'IA cherche d'abord les passages utiles dans nos documents, puis répond à partir d'eux.
- **Jeton** : un petit morceau de texte produit par le modèle.

## Si on te pose la question
- **"Lequel on choisit ?"** Ça dépend de la tâche, et c'est le but de l'outil. Rapide pour le tri et le routage, Qwen pour les documents et le raisonnement, sur ce qu'on a testé.
- **"C'est fiable, ces chiffres ?"** Deux passages complets pour l'instant, donc c'est une première indication. Les petits écarts restent à confirmer, et chaque nouveau test affine les chiffres.
- **"Ce sont nos vraies données ?"** Non, le corpus est fictif. Il contient de vrais formats : Word, PDF, Excel, PowerPoint, mails.
- **"Et si le modèle se trompe sur un calcul ?"** Il arrive aux deux de se tromper : le score Calcul reste bas chez les deux, autour de 50. Ces modèles ne sont pas à laisser seuls sur ce type de tâche.
- **"La suite ?"** Comparer plusieurs machines, mesurer la consommation d'énergie, ajouter une connexion sécurisée, tester sur de vrais documents.

## Si ça coince
- Page figée ou vide : recharger, ou passer sur l'onglet Résultats.
- "Serveur injoignable" : attendre quelques secondes et recharger.
- Le Chat est bloqué pendant un Benchmark : c'est voulu, pour ne pas fausser les temps.
