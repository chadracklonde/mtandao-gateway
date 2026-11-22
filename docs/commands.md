# Exécution des commandes Cloud

Le Gateway interroge le Cloud (connexion sortante uniquement), exécute les commandes reçues sur le routeur MikroTik et renvoie le résultat. L'exécution est **désactivée par défaut** : elle permet au Cloud de modifier l'état du routeur.

## Activation

Variables d'environnement (voir `packaging/linux/gateway.env.example`) :

| Variable | Rôle |
| --- | --- |
| `GATEWAY_COMMANDS_ENABLED=true` | active l'exécution (désactivée sinon) |
| `CLOUD_API_URL`, `GATEWAY_TOKEN_FILE` | connexion Cloud, comme pour le heartbeat |
| `ROUTEROS_CONFIG_FILE` | connexion locale au routeur |
| `GATEWAY_STATE_DIR` | dossier du registre (défaut `/var/lib/mtandao-gateway`, mode 0700) |
| `GATEWAY_COMMAND_POLL_INTERVAL` | sondage en secondes, de 2 à 300 (10 par défaut) |
| `GATEWAY_SITE_ID`, `GATEWAY_ROUTER_ID` | facultatifs : refuse toute commande destinée à un autre site ou routeur |

## Opérations

| Opération | Effet RouterOS | Payload |
| --- | --- | --- |
| `health.check` | lecture : identité, version, carte, uptime, interfaces | aucun |
| `hotspot.profile.list` | `/ip/hotspot/user/profile/print` | aucun |
| `hotspot.active-sessions.list` | `/ip/hotspot/active/print` | aucun |
| `hotspot.voucher.create` | `/ip/hotspot/user/add` (nom = mot de passe = code) | `code`, `profile` (opt.), `duration_minutes` (opt.), `data_limit_mb` (opt.), `comment` (opt.) |
| `hotspot.voucher.disable` | désactive l'utilisateur et termine ses sessions actives | `code` |

Limites posées sur le ticket, comme le fait l'application Mtandao Ticket :

- `duration_minutes` → `limit-uptime` : limite de temps d'utilisation, qui court à partir de la première connexion. Absent : pas de limite.
- `data_limit_mb` → `limit-bytes-total` : limite de données (1 Mo = 1 048 576 octets, comme l'application). Absent : pas de limite.
- `comment` → commentaire du ticket au format `vc-JJMMAA-HHMM <note>` (ou `up-…`). Avec le script on-login du profil de l'application ou de Mikhmon, ce préfixe est ce qui fait enregistrer la vente, marquer le ticket « utilisé » et armer son expiration ; sans lui, seule la limite du routeur s'applique. Les tickets créés par le Cloud portent ce commentaire.

Le résultat renvoyé indique les limites appliquées (`usage_limit_minutes`, `data_limit_mb`, 0 = aucune).

Le code d'un voucher doit correspondre à `[A-Za-z0-9_.-]{1,64}` et un profil à `[A-Za-z0-9 _.-]{1,120}`. Le Gateway est volontairement plus strict que le Cloud : une commande hors de ces formats est rejetée (`invalid_payload`).

## Garanties

- **Liste blanche stricte** : chaque requête RouterOS est vérifiée mot par mot (commande, attributs, filtres, formats de valeurs) avant l'envoi. Aucune commande libre, aucun attribut non prévu.
- **Une exécution au plus** : toute commande reçue est écrite dans un registre durable (`commands.json`, 0600) *avant* d'être exécutée. Une commande redistribuée renvoie le résultat déjà enregistré sans retoucher le routeur.
- **Pas de rejeu des écritures** : si le Gateway s'arrête pendant une création ou une désactivation, la commande est rapportée en échec `interrupted` et jamais rejouée automatiquement. Les lectures, sans effet, sont relancées.
- **Reprise après panne** : les commandes déjà reçues mais non exécutées sont exécutées au redémarrage ; les résultats non acquittés par le Cloud sont renvoyés jusqu'à acceptation ou refus définitif.
- **Pas de fuite d'erreur interne** : le Cloud ne reçoit que des codes stables et des messages génériques, jamais d'adresse, d'identifiant ou de détail TLS.

## Codes d'échec

`invalid_command`, `expired`, `wrong_target`, `invalid_payload`, `router_unavailable`, `voucher_exists`, `voucher_not_found`, `interrupted`, `result_too_large`. L'erreur envoyée au Cloud a la forme `code: message`.

## Limites

- Les listes sont plafonnées (500 lignes et environ 40 Ko) ; le résultat indique `truncated: true`.
- Le Gateway ne vérifie pas de signature de commande (le Cloud n'en émet pas encore) : l'intégrité repose sur TLS et le jeton du Gateway.
- Le registre contient les codes de voucher : protéger le dossier d'état et ses sauvegardes.
- Validé avec un routeur simulé uniquement ; voir [le plan de test en laboratoire](lab-test-plan.md).
