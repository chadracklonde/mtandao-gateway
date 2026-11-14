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
- `docs` : protocole et procédures d'installation.

## Sécurité

La passerelle n'expose aucun port d'administration MikroTik sur Internet. Elle initie une connexion sortante vers Mtandao Cloud. Les commandes sont limitées à une liste blanche, authentifiées, signées, expirables et protégées contre le rejeu.

Les secrets RouterOS sont conservés localement dans un stockage protégé et ne sont jamais envoyés au tableau de bord ni inclus dans les journaux.

## État

Phase initiale : contrats et architecture. Aucune commande réelle n'est encore exécutée sur un routeur. Les opérations RouterOS devront être développées et testées sur un équipement de laboratoire avant toute utilisation de production.
