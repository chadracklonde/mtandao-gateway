# Connexion locale à MikroTik RouterOS

## État de l'implémentation

Le Gateway dispose d'un transport RouterOS API sur TLS et d'une interface strictement en lecture seule pour :

- les ressources système (/system/resource/print) ;
- l'identité (/system/identity/print) ;
- les interfaces (/interface/print).

Le transport refuse les noms DNS et les adresses IP publiques. Il accepte uniquement les adresses IP privées, loopback ou link-local. Il n'expose pas d'API permettant d'exécuter une commande RouterOS arbitraire.

## Préparation du routeur

1. Activer le service API-SSL de RouterOS sur le port TCP 8729.
2. Installer un certificat TLS approuvé par le système qui exécute le Gateway. Le nom/IP du certificat doit correspondre à l'adresse de connexion.
3. Créer un compte dédié au Gateway, avec le minimum de droits requis pour consulter les informations système et les interfaces.
4. Autoriser le port API-SSL uniquement depuis l'adresse locale du Gateway dans le pare-feu du routeur.
5. Ne jamais publier le port 8729 sur Internet et ne pas utiliser le port API non chiffré 8728.

## Limites et sécurité

Le transport utilise TLS avec une version minimale TLS 1.2 et vérifie le certificat du serveur. Aucun mode de désactivation de vérification TLS n'est fourni. Les identifiants sont obligatoires et ne doivent jamais être inscrits dans le dépôt ou les journaux.

Les tests actuels utilisent un client simulé. Une validation sur un MikroTik de laboratoire reste nécessaire avant tout déploiement. Les opérations d'écriture, la gestion des utilisateurs Hotspot et l'exécution de commandes reçues du Cloud ne sont pas activées.

## Configuration du Gateway

Créer un fichier JSON local, appartenant à l'utilisateur système du Gateway et accessible uniquement à cet utilisateur (mode 0600) :

```json
{
  "address": "192.168.88.1:8729",
  "username": "mtandao-gateway",
  "password": "REMPLACER_PAR_UN_SECRET",
  "timeout_seconds": 8
}
```

Protéger le répertoire parent (mode 0700), puis définir :

```sh
export ROUTEROS_CONFIG_FILE="/etc/mtandao-gateway/router.json"
export ROUTEROS_POLL_INTERVAL="30"
```

Le Gateway actualise les informations en lecture seule toutes les 30 secondes par défaut. Le point local `GET /health` inclut un instantané RouterOS sans exposer l'adresse, le nom d'utilisateur ni le mot de passe. En cas d'échec, l'état indique seulement que le routeur est indisponible.

Le fichier est refusé s'il s'agit d'un lien symbolique, s'il n'est pas régulier, si ses permissions sont trop ouvertes ou si l'adresse cible n'est pas une IP locale. Ne pas placer ce fichier dans Git.
