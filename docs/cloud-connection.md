# Connexion Cloud du Gateway

## Enrôlement initial

Le Gateway dispose de la commande `gateway enroll`, qui échange le jeton temporaire contre un jeton permanent et l'enregistre sans l'afficher.

Préparer deux chemins distincts :

- `GATEWAY_ENROLLMENT_TOKEN_FILE` : fichier contenant le jeton temporaire fourni lors de l'enregistrement du routeur dans Mtandao Cloud.
- `GATEWAY_TOKEN_FILE` : destination du jeton permanent. Le fichier ne doit pas déjà exister.

Le fichier temporaire doit être régulier, non symbolique et protégé en mode 0600. Exemple :

```sh
sudo install -d -m 700 /etc/mtandao-gateway
# Déposer le jeton temporaire dans ce fichier par un mécanisme sécurisé.
sudo chmod 600 /etc/mtandao-gateway/enrollment.token

export CLOUD_API_URL="https://api.example.invalid"
export GATEWAY_ENROLLMENT_TOKEN_FILE="/etc/mtandao-gateway/enrollment.token"
export GATEWAY_TOKEN_FILE="/etc/mtandao-gateway/gateway.token"

./gateway enroll
```

À la réussite, le programme affiche uniquement les identifiants du routeur et du site, stocke le jeton permanent dans le fichier cible (répertoire 0700, fichier 0600), puis supprime le fichier du jeton temporaire. Si la destination existe déjà, la commande s'arrête avant de consommer le jeton temporaire. Si le stockage échoue après l'échange Cloud, le jeton permanent ne peut pas être récupéré : il faut générer un nouvel enrôlement depuis Cloud.

L'endpoint appelé est `POST /v1/gateway/enroll`. Le jeton temporaire est à usage unique et expire après 15 minutes. Le client exige HTTPS en production et ne journalise jamais les secrets.

## Configuration du heartbeat

Pour activer les heartbeats Cloud, configurer ensemble :

- `CLOUD_API_URL` : URL racine de l'API Mtandao Cloud, obligatoirement HTTPS en production.
- `GATEWAY_TOKEN_FILE` : fichier contenant le jeton permanent.
- `GATEWAY_HEARTBEAT_INTERVAL` : intervalle facultatif en secondes, de 15 à 3600 (60 par défaut).

Exemple :

```sh
export CLOUD_API_URL="https://api.example.invalid"
export GATEWAY_TOKEN_FILE="/etc/mtandao-gateway/gateway.token"
export GATEWAY_HEARTBEAT_INTERVAL="60"
./gateway
```

Ne jamais inscrire les secrets dans le dépôt, une image de conteneur, une commande shell historisée ou un journal. Les fichiers doivent appartenir à l'utilisateur système du service et être inaccessibles aux autres utilisateurs.

## Fonctionnement

- Un heartbeat est envoyé immédiatement au démarrage, puis toutes les 60 secondes par défaut.
- Endpoint : `POST /v1/gateway/heartbeat`.
- Le secret est transmis uniquement dans l'en-tête Authorization sur HTTPS.
- Les erreurs réseau déclenchent une reprise exponentielle (5 secondes à 5 minutes) avec jitter.
- Une réponse 401/403 suspend les tentatives fréquentes : le Gateway attend cinq minutes et nécessite un jeton remplacé par l'administrateur.
- Le délai HTTP est limité et les signaux SIGINT/SIGTERM arrêtent proprement le worker.
- Les journaux n'affichent jamais le jeton.

Le mode local seul reste disponible lorsque les variables Cloud ne sont pas configurées. Le service `/health` indique que le processus répond, pas que le Cloud ou le routeur MikroTik est joignable.

## Limites actuelles

L'enrôlement CLI, l'échange Cloud et le stockage local protégé sont maintenant implémentés. Restent à faire : rotation du jeton, réception et authentification des commandes, puis opérations RouterOS. Le déploiement de l'API et l'application des migrations Supabase restent des opérations distinctes.
