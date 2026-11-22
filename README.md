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
- Enrôlement Cloud via `gateway enroll` avec échange de jeton à usage unique.
- Stockage atomique du jeton permanent (répertoire 0700, fichier 0600).
- Client HTTP sortant pour le heartbeat Mtandao Cloud.
- Interface RouterOS en lecture seule (ressources système, identité et interfaces), testée avec transport simulé.
- Transport RouterOS API sur TLS, limité aux adresses IP privées, loopback ou link-local et à trois commandes de lecture fixes.
- Configuration RouterOS locale protégée (JSON, permissions strictes) et surveillance périodique en lecture seule exposée dans `/health`.
- Authentification par jeton bearer conservé dans un fichier protégé.
- Reconnexion avec backoff exponentiel et arrêt contrôlé.
- Réception et exécution des commandes Cloud (désactivées par défaut) : liste blanche stricte, registre durable, exécution unique, reprise après panne. Voir [l'exécution des commandes](docs/commands.md).
- Adaptateur Hotspot RouterOS (profils, sessions actives, création et désactivation de vouchers).
- Confiance au certificat du routeur via `ca_file` (vérification TLS jamais désactivée).
- Service systemd durci : `packaging/linux/`.

Voir [la configuration de connexion Cloud](docs/cloud-connection.md).

## Sécurité

La passerelle n'expose aucun port d'administration MikroTik sur Internet. Elle initie une connexion sortante vers Mtandao Cloud. Les commandes sont limitées à une liste blanche, authentifiées, signées, expirables et protégées contre le rejeu.

Les secrets RouterOS sont conservés localement dans un stockage protégé et ne sont jamais envoyés au tableau de bord ni inclus dans les journaux.

## État

L'enrôlement CLI, le stockage protégé du jeton permanent et le client de heartbeat Cloud sont implémentés. Le transport TLS RouterOS API est implémenté, mais doit encore être validé sur un équipement de laboratoire avec un certificat approuvé. L'exécution des commandes est implémentée et testée avec un routeur simulé et contre l'API Cloud, mais pas encore validée sur un équipement réel (voir [le plan de test](docs/lab-test-plan.md)). L'authentification cryptographique des commandes reste à développer. Aucune commande arbitraire ne peut être exécutée. Le heartbeat confirme la connexion au Cloud, pas la disponibilité du routeur local.

## Documentation

- [Cahier des charges Mtandao Gateway](docs/cahier-des-charges.md).
- [Connexion Cloud](docs/cloud-connection.md).
- [Architecture](docs/architecture.md).
- [Protocole de commande](docs/protocol.md).
- [Exécution des commandes](docs/commands.md) et [plan de test en laboratoire](docs/lab-test-plan.md).
- [Guide de développement](docs/development.md).
- [Cahier des charges Mtandao Cloud](https://github.com/chadracklonde/mtandao-cloud/blob/main/docs/cahier-des-charges.md).
- [Cahier des charges Mtandao Ticket](https://github.com/chadracklonde/mtandao-ticket-app/blob/main/docs/cahier-des-charges.md).
