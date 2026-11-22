# Protocole de commande Gateway

Le contrat JSON de commande est défini dans `packages/contracts/gateway-command.schema.json` du dépôt Cloud (résultat : `gateway-command-result.schema.json`); ce dépôt conserve cette documentation et doit rester compatible avec la version publiée du contrat.

## Champs essentiels

- `command_id` : UUID unique pour déduplication.
- `site_id`, `gateway_id`, `router_id` : périmètre et destinataire.
- `operation` : opération autorisée, jamais une commande shell/CLI libre.
- `issued_at`, `expires_at` : fenêtre de validité.
- `payload` : arguments validés par schéma.
- `signature` : signature calculée sur une représentation canonique du message.

## Réponse

Chaque résultat doit inclure `command_id`, `gateway_id`, `status` (`succeeded`, `failed`, `rejected`, `expired`), `started_at`, `completed_at`, un code d'erreur stable et un résultat expurgé des secrets.

## Idempotence

Le Gateway mémorise les `command_id` traités. Une commande répétée renvoie le résultat précédent ou un état en cours; elle ne doit pas provoquer une seconde création de voucher.

## Liste blanche initiale

- `health.check`
- `hotspot.voucher.create`
- `hotspot.voucher.disable`
- `hotspot.profile.list`
- `hotspot.active-sessions.list`

Chaque opération nécessite un schéma de payload précis et des tests. La liste ne signifie pas que les opérations sont déjà implémentées.
