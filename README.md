# Mtandao Gateway

Passerelle locale entre Mtandao Cloud et les routeurs MikroTik RouterOS.

## Cibles

- Linux : service permanent pour mini-PC ou ordinateur.
- Android : service adapté aux restrictions d'arrière-plan du système.
- Mode téléphone : l'application Mtandao Ticket peut fournir une passerelle temporaire.

## Architecture prévue

- `core` : registre des commandes, validation, état et journal local.
- `adapters/routeros` : abstraction RouterOS API.
- `transports/cloud` : connexion sortante sécurisée au Cloud.
- `platforms/linux` et `platforms/android` : intégration système.

## Fonctionnalités présentes

- Endpoint local `GET /health`.
- Client HTTP sortant pour le heartbeat Mtandao Cloud.
- Authentification par jeton bearer conservé dans un fichier protégé.
- Reconnexion avec backoff exponentiel et arrêt contrôlé.

Voir [la configuration de connexion Cloud](docs/cloud-connection.md).

## Sécurité

La passerelle n'expose aucun port d'administration MikroTik sur Internet. Elle initie une connexion sortante vers Mtandao Cloud. Les commandes sont limitées à une liste blanche, authentifiées, signées, expirables et protégées contre le rejeu.

Les secrets RouterOS sont conservés localement dans un stockage protégé et ne sont jamais envoyés au tableau de bord ni inclus dans les journaux.

## État

Le client de heartbeat Cloud est implémenté. L'enrôlement initial depuis le binaire Go, la réception des commandes et l'exécution RouterOS restent à développer et tester sur un équipement de laboratoire. Le heartbeat confirme la connexion au Cloud, pas la disponibilité du routeur local.

## Documentation

- [Cahier des charges Mtandao Gateway](docs/cahier-des-charges.md).
- [Connexion Cloud](docs/cloud-connection.md).
- [Architecture](docs/architecture.md).
- [Protocole de commande](docs/protocol.md).
- [Guide de développement](docs/development.md).
- [Cahier des charges Mtandao Cloud](https://github.com/chadracklonde/mtandao-cloud/blob/main/docs/cahier-des-charges.md).
- [Cahier des charges Mtandao Ticket](https://github.com/chadracklonde/mtandao-ticket-app/blob/main/docs/cahier-des-charges.md).
