# Plan de test en laboratoire RouterOS

Les tests automatiques utilisent un routeur simulé. Avant tout déploiement, valider sur de vrais équipements. Statut : **non exécuté**.

## Matrice

| Cas | RouterOS 6 (≥ 6.43) | RouterOS 6 (< 6.43) | RouterOS 7 |
| --- | --- | --- | --- |
| Connexion API-SSL (port 8729) avec CA configurée | à tester | à tester | à tester |
| Authentification | méthode directe | méthode à défi (prise en charge par la bibliothèque, non testée) | méthode directe |
| `health.check` | à tester | à tester | à tester |
| `hotspot.profile.list` | à tester | à tester | à tester |
| `hotspot.active-sessions.list` | à tester | à tester | à tester |
| `hotspot.voucher.create` (avec/sans profil, avec/sans durée) | à tester | à tester | à tester |
| `hotspot.voucher.disable` (avec session active) | à tester | à tester | à tester |

## Préparation

1. Créer un serveur Hotspot de test et un profil.
2. Activer `api-ssl` avec un certificat dont l'IP figure dans les SAN, exporter le certificat (ou la CA) et le référencer par `ca_file` dans `router.json`. Ne jamais désactiver la vérification TLS.
3. Créer un compte dédié avec uniquement les droits nécessaires : lecture système/interfaces, gestion des utilisateurs Hotspot et des sessions actives.
4. Limiter le port 8729 à l'adresse du Gateway dans le pare-feu du routeur.

## Points à vérifier

- Le format `limit-uptime` généré (`1d2h30m`) est accepté et appliqué.
- La désactivation termine bien la session d'un client connecté.
- Création d'un code déjà existant : échec `voucher_exists`, utilisateur existant inchangé.
- Routeur injoignable ou certificat invalide : échec `router_unavailable`, sans détail dans le message.
- Redémarrage du Gateway pendant une écriture : échec `interrupted`, aucune seconde création.
- Un certificat auto-signé non déclaré dans `ca_file` est refusé.
- Aucun secret (mot de passe, jeton) dans les journaux ni dans les résultats.
- Compatibilité des noms de propriétés entre RouterOS 6 et 7 pour `/ip/hotspot/active/print` (`session-time-left`, `bytes-in`, `bytes-out`).
