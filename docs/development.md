# Guide de développement

## Prérequis

- Go 1.22 ou version compatible.
- Accès à un routeur de laboratoire pour les futurs tests RouterOS.

## Qualité attendue

- Tests unitaires pour validation, signature, expiration et déduplication.
- Tests d'intégration avec simulateur ou routeur de laboratoire.
- Tests de compatibilité RouterOS 6 et 7.
- Tests réseau interrompu/repris.
- Analyse statique et dépendances verrouillées.
- Aucune donnée sensible dans les fixtures ou logs.

## Étapes réalisées

- Squelette du service Go et endpoint local `GET /health`.
- Client HTTP Cloud pour `POST /v1/gateway/heartbeat`.
- Vérification HTTPS en production, jeton bearer au format attendu et réponse Cloud.
- Heartbeat immédiat, intervalle configurable, reprise exponentielle avec jitter.
- Arrêt contrôlé du worker et protection des permissions du fichier de jeton.

Voir [Connexion Cloud](cloud-connection.md) pour la configuration.

## Critères de première version

1. Enrôlement sécurisé d'un Gateway.
2. Heartbeat et révocation.
3. Connexion sortante au Cloud.
4. Exécution de `health.check` sur routeur de test.
5. Réception et acquittement idempotents.
6. Journal local consultable et expurgé.
