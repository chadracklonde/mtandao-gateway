# Guide de développement

## Prérequis

Les choix de langage et de framework de la passerelle seront arrêtés après un prototype de compatibilité RouterOS et une comparaison des contraintes Linux/Android.

## Qualité attendue

- Tests unitaires pour validation, signature, expiration et déduplication.
- Tests d'intégration avec simulateur ou routeur de laboratoire.
- Tests de compatibilité RouterOS 6 et 7.
- Tests réseau interrompu/repris.
- Analyse statique et dépendances verrouillées.
- Aucune donnée sensible dans les fixtures ou logs.

## Critères de première version

1. Enrôlement sécurisé d'un Gateway.
2. Heartbeat et révocation.
3. Connexion sortante au Cloud.
4. Exécution de `health.check` sur routeur de test.
5. Réception et acquittement idempotents.
6. Journal local consultable et expurgé.
