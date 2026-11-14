# Architecture Mtandao Gateway

## Rôle

Le Gateway relie un espace de site Mtandao Cloud à un ou plusieurs routeurs locaux. Il peut fonctionner sur Linux ou Android. Le téléphone Mtandao Ticket reste une passerelle temporaire possible.

## Flux

1. Le Gateway s'enregistre auprès du Cloud avec un jeton d'enrôlement à usage limité.
2. Il établit une connexion sortante TLS.
3. Il reçoit une commande liée à son site et à un routeur autorisé.
4. Il vérifie signature, expiration, identifiant unique, permissions et schéma.
5. Il exécute uniquement une opération de la liste blanche via l'adaptateur RouterOS.
6. Il renvoie un résultat corrélé au `command_id`.
7. Il conserve localement un journal et une file de reprise.

## Sécurité obligatoire

- TLS et identité propre à chaque appareil.
- Préférer mTLS ou jetons courts renouvelables.
- Signature des commandes côté Cloud; vérification côté Gateway.
- Expiration courte et protection anti-rejeu par `command_id`.
- Liste blanche stricte des opérations; jamais de CLI arbitraire.
- Limites de débit, délais d'attente et validation de payload.
- Stockage local chiffré des secrets RouterOS.
- Journaux expurgés de tout secret.
- Révocation distante d'un Gateway.
- Aucune redirection publique des ports RouterOS.

## Compatibilité RouterOS

L'adaptateur principal utilisera RouterOS API, en tenant compte des installations RouterOS 6 et 7. L'API REST RouterOS 7 peut être un adaptateur facultatif ultérieur. Les versions et capacités seront détectées, pas supposées.

## Hors connexion

Les opérations locales permises peuvent continuer lorsque le Cloud est indisponible. Les commandes Cloud reçues mais non exécutables expirent et ne doivent pas être exécutées tardivement sans nouvelle validation. Les résultats sont conservés dans une outbox locale avec identifiants idempotents.

## Plateformes

- Linux : service supervisé par systemd, configuration protégée, mises à jour contrôlées.
- Android : service foreground lorsque requis par Android, notification persistante si nécessaire, gestion explicite des restrictions batterie et réseau.
