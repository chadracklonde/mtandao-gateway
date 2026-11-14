# Connexion Cloud du Gateway

## Configuration

Le Gateway peut démarrer en mode local seul. Pour activer les heartbeats Cloud, configurer ensemble :

- `CLOUD_API_URL` : URL racine de l'API Mtandao Cloud, obligatoirement HTTPS en production.
- `GATEWAY_TOKEN_FILE` : chemin d'un fichier contenant le jeton permanent retourné par l'enrôlement.
- `GATEWAY_HEARTBEAT_INTERVAL` : intervalle facultatif en secondes, de 15 à 3600 (60 par défaut).

Exemple Linux :

```sh
sudo install -d -m 700 /etc/mtandao-gateway
sudo install -m 600 /dev/null /etc/mtandao-gateway/gateway.token
# Inscrire le jeton permanent dans ce fichier par un mécanisme sécurisé.
sudo chmod 600 /etc/mtandao-gateway/gateway.token

export CLOUD_API_URL="https://api.example.invalid"
export GATEWAY_TOKEN_FILE="/etc/mtandao-gateway/gateway.token"
export GATEWAY_HEARTBEAT_INTERVAL="60"
```

Ne jamais inscrire le jeton dans le dépôt, une image de conteneur, une commande shell historisée ou un journal. Le fichier doit appartenir à l'utilisateur système du service et être inaccessible aux autres utilisateurs (mode 0600).

## Fonctionnement

- Un heartbeat est envoyé immédiatement au démarrage, puis toutes les 60 secondes par défaut.
- Endpoint : `POST /v1/gateway/heartbeat`.
- Le secret est transmis uniquement dans l'en-tête Authorization sur HTTPS.
- Les erreurs réseau déclenchent une reprise exponentielle (5 secondes à 5 minutes) avec jitter.
- Une réponse 401/403 suspend les tentatives fréquentes : le Gateway attend cinq minutes et nécessite un jeton remplacé/révoqué par l'administrateur.
- Le délai HTTP est limité et les signaux SIGINT/SIGTERM arrêtent proprement le worker.
- Les journaux n'affichent jamais le jeton.

Le mode local seul reste disponible lorsque les deux variables Cloud sont absentes. Si une seule est définie, le démarrage échoue pour éviter une configuration partielle.

## Limites actuelles

Cette étape implémente le client sortant et le heartbeat. Elle ne réalise pas encore l'enrôlement initial depuis le binaire Go, la rotation automatique du jeton, la réception de commandes, ni les opérations RouterOS. Le service `/health` indique que le processus répond, pas que le Cloud ou le routeur MikroTik est joignable.
