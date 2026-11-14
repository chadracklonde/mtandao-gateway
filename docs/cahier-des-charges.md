# Mtandao Gateway — Cahier des charges fonctionnel et technique

- Version : 0.1
- Date : 14 novembre 2026
- Dépôt : chadracklonde/mtandao-gateway
- Statut : cadrage initial aligné sur Mtandao Cloud v0.1
- Référentiel associé : docs/cahier-des-charges.md dans mtandao-cloud

## 1. Objet et mission

Mtandao Gateway est un agent logiciel installé dans le réseau local d'une Wi-Fi zone. Il assure le lien sécurisé entre Mtandao Cloud et un ou plusieurs routeurs MikroTik. Il reçoit des commandes métier autorisées depuis le Cloud, les traduit en opérations RouterOS contrôlées, puis renvoie les résultats.

Il ne remplace ni RouterOS ni Mtandao Ticket. Il complète le mode local et permet l'exécution de tâches distantes même lorsque le téléphone du propriétaire est éteint.

## 2. Plateformes et modes de déploiement

- Linux : service permanent supervisé par systemd, destiné à un mini-PC ou ordinateur du site.
- Android : agent fonctionnant selon les règles de l'OS, avec service foreground lorsque nécessaire et gestion des restrictions batterie.
- Téléphone Mtandao Ticket : passerelle temporaire distincte, disponible uniquement lorsque l'application et son service sont actifs.
- Un site peut associer plusieurs routeurs et prévoir une passerelle principale et une passerelle de secours, sous réserve d'un mécanisme empêchant l'exécution concurrente d'une même commande.

Le Gateway permanent est développé en Go. Le choix de bibliothèques doit être justifié, versionné et maintenu par des dépendances verrouillées.

## 3. Objectifs

- S'enrôler auprès de Mtandao Cloud à l'aide d'un code ou jeton à usage unique.
- Obtenir une identité et un secret propres à l'appareil.
- Maintenir une connexion sortante sécurisée au Cloud sans ouvrir les ports RouterOS sur Internet.
- Envoyer périodiquement un heartbeat et signaler son état.
- Recevoir uniquement les commandes autorisées pour son site et ses routeurs associés.
- Exécuter les opérations via l'API RouterOS et retourner un résultat corrélé.
- Continuer les opérations locales autorisées lorsque le Cloud est temporairement indisponible.
- Conserver une file locale, dédupliquer les commandes et reprendre proprement après redémarrage.
- Permettre la révocation et la rotation des identifiants.

## 4. Hors périmètre

- Exposer RouterOS API ou WinBox sur Internet.
- Fournir un shell distant, une console CLI libre ou l'exécution de scripts arbitraires.
- Remplacer les fonctions de gestion locale de Mtandao Ticket.
- Confirmer un paiement ou décider de la comptabilité : ces décisions relèvent du Cloud.
- Exécuter une commande expirée uniquement parce que la connexion revient.
- Permettre à l'IA d'envoyer directement des commandes réseau.

## 5. Architecture logique

Composants internes :
- **cloud client** : connexion sortante TLS, authentification, heartbeat, réception des commandes et accusés.
- **enrollment** : échange du jeton temporaire et stockage sécurisé de l'identité.
- **command dispatcher** : validation du site, du routeur, de l'opération, de la signature, de l'expiration et de l'identifiant.
- **allowlist** : registre fermé des opérations prises en charge et de leurs schémas.
- **routeros adapter** : client RouterOS API/API-SSL, détection des versions et capacités.
- **local store** : configuration protégée, journal et outbox/inbox persistants.
- **observability** : logs structurés expurgés, métriques et diagnostics.

Le Gateway initie toutes les connexions vers le Cloud. Les connexions vers les routeurs sont locales au réseau du site.

## 6. Cycle d'enrôlement et identité

1. Un utilisateur autorisé enregistre un routeur et génère un jeton d'enrôlement à durée courte.
2. L'installateur configure le Gateway et saisit/scanne le jeton.
3. Le Gateway échange ce jeton une seule fois contre un secret permanent.
4. Le Cloud stocke uniquement l'empreinte du secret permanent.
5. Le Gateway conserve le secret dans le stockage sécurisé du système et ne le journalise jamais.
6. Une révocation doit empêcher l'authentification et les nouvelles commandes.
7. La rotation doit permettre une transition contrôlée sans fenêtre d'accès non autorisée.

Le mécanisme actuellement amorcé dans mtandao-cloud utilise un jeton temporaire à usage unique, une empreinte SHA-256 en base et un secret aléatoire de 256 bits retourné une seule fois. Le heartbeat par bearer token est une première étape ; mTLS et la rotation restent des exigences de durcissement avant production.

## 7. Présence et heartbeat

- Envoyer un heartbeat environ toutes les 60 secondes, avec jitter.
- Le Cloud met à jour last_seen_at après authentification valide.
- Le seuil initial d'inactivité est de trois minutes ; au-delà, le Gateway peut être déclaré offline par un ordonnanceur de confiance.
- Les états suspended et revoked sont terminaux du point de vue du heartbeat : un signal ne doit jamais les réactiver.
- En cas d'indisponibilité Internet, appliquer un backoff exponentiel et reprendre sans boucle agressive.
- Ne jamais confondre perte de connexion Cloud et indisponibilité du routeur local.

## 8. Protocole de commande

Chaque commande doit contenir au minimum :
- command_id : UUID stable et unique ;
- site_id, gateway_id et router_id ;
- operation : nom d'une opération autorisée ;
- issued_at et expires_at ;
- schema_version ;
- payload validé selon le schéma de l'opération ;
- signature ou mécanisme d'authenticité défini par le protocole Cloud.

Le Gateway vérifie l'authenticité, la portée tenant, le routeur associé, l'expiration, la version de schéma et l'autorisation de l'opération avant exécution.

Réponse attendue : command_id, gateway_id, status (succeeded, failed, rejected ou expired), started_at, completed_at, code d'erreur stable et résultat expurgé.

## 9. Liste blanche initiale

Opérations candidates, chacune à implémenter et tester séparément :
- health.check
- hotspot.voucher.create
- hotspot.voucher.disable
- hotspot.profile.list
- hotspot.active-sessions.list

La présence dans cette liste ne signifie pas que l'opération est déjà implémentée. Chaque opération doit avoir un DTO, un schéma de validation, une permission, un délai maximal et des tests RouterOS 6/7.

Aucune commande shell, script RouterOS arbitraire ou expression libre ne doit être acceptée.

## 10. Compatibilité RouterOS

- RouterOS API binaire est le protocole principal de compatibilité RouterOS 6 et 7.
- API-SSL est préféré lorsque disponible et configuré correctement.
- REST RouterOS 7 est un adaptateur facultatif ultérieur.
- Détecter version, architecture et capacités ; ne pas supposer qu'une commande existe partout.
- Utiliser un compte RouterOS dédié, avec privilèges minimaux.
- Les secrets routeur restent chiffrés dans le stockage local du Gateway.
- Les erreurs de compatibilité sont explicites et ne doivent pas déclencher de commande de remplacement non validée.

## 11. Fonctionnement hors connexion et reprise

- La perte du Cloud ne doit pas interrompre une opération locale déjà autorisée et sûre.
- Les commandes Cloud non reçues ne sont pas exécutées.
- Les commandes expirées sont rejetées au retour de connexion.
- Les résultats sont conservés dans une outbox locale jusqu'à accusé de réception.
- Les command_id déjà traités sont dédupliqués ; une répétition renvoie le résultat connu sans refaire l'opération.
- Une commande dont le résultat est incertain après une coupure doit être rapprochée avec RouterOS avant toute répétition.
- La persistance doit survivre à un redémarrage du service ou de l'appareil.

## 12. Sécurité et confidentialité

- TLS obligatoire pour toutes les connexions Cloud.
- mTLS recommandé pour l'identité durable en production.
- Secrets conservés dans Android Keystore ou un magasin de secrets Linux approprié ; permissions système minimales.
- Aucun secret dans les logs, traces, crash reports, exemples ou dépôts.
- Validation stricte de toutes les entrées, limites de taille et délais d'attente.
- Anti-rejeu, expiration, idempotence et limitation de débit.
- Révocation distante, rotation des credentials et audit des opérations sensibles.
- Aucun port RouterOS publié sur WAN.
- Aucun accès inter-sites : le Gateway est limité au site et aux routeurs explicitement associés.

## 13. Observabilité et exploitation

- Logs structurés avec identifiants de corrélation, sans données sensibles.
- État du service, version, uptime, dernière connexion Cloud, dernière activité routeur et profondeur des files.
- Diagnostic exportable et expurgé.
- Installation et mise à jour contrôlées, vérification d'intégrité des binaires et mécanisme de retour arrière.
- Linux : unité systemd, redémarrage supervisé et configuration protégée.
- Android : notification persistante si le service foreground est requis, gestion batterie et état réseau visible.

## 14. Tests et critères d'acceptation

1. Jeton d'enrôlement valide accepté une seule fois ; jeton expiré ou réutilisé refusé.
2. Secret permanent non présent en clair dans la base ni dans les logs.
3. Gateway suspendu ou révoqué incapable de heartbeat et de recevoir une commande.
4. Heartbeat régulier actualisant la présence ; Gateway silencieux déclaré offline après le seuil configuré.
5. Commande signée, autorisée et non expirée exécutée sur un routeur de test.
6. Commande altérée, hors site, non autorisée ou expirée rejetée.
7. Même command_id envoyé deux fois sans double exécution.
8. Coupure réseau pendant une opération : résultat rapproché avant reprise.
9. Reconnexion Cloud avec envoi des résultats en attente une seule fois.
10. Tests de compatibilité RouterOS 6 et 7 sur routeur réel ou laboratoire représentatif.
11. Redémarrage Linux/Android sans perte d'identité ni de file persistante.
12. Vérification qu'aucun port d'administration MikroTik n'est exposé sur Internet.

## 15. Lots de réalisation

- Lot 0 : spécification du protocole et contrats partagés avec Cloud.
- Lot 1 : configuration, stockage sécurisé et enrôlement.
- Lot 2 : heartbeat, révocation et état en ligne/hors ligne.
- Lot 3 : connexion sortante persistante et transport de commandes.
- Lot 4 : dispatcher, allowlist, signature, expiration et idempotence.
- Lot 5 : adaptateur RouterOS API pour health.check.
- Lot 6 : vouchers, profils et sessions selon les permissions.
- Lot 7 : outbox, reprise, observabilité et mises à jour.
- Lot 8 : packaging Linux/systemd puis Android et tests terrain.

Chaque lot doit être accompagné de tests exécutés, résultats vérifiables et documentation mise à jour.

## 16. Dépendances avec les autres dépôts

- Mtandao Ticket : https://github.com/chadracklonde/mtandao-ticket-app — application locale et passerelle téléphone temporaire.
- Mtandao Cloud : https://github.com/chadracklonde/mtandao-cloud — identité, sites, API, enrôlement, présence, commandes et synchronisation.
- Contrats de commandes et d'événements : dépôt Cloud, packages/contracts ; toute évolution doit rester compatible ou être versionnée.

## 17. Gouvernance

Ce document décrit les exigences du Gateway, pas l'état d'avancement réel du code. Les guides architecture, protocole et développement précisent l'implémentation. Une fonction n'est considérée livrée qu'après code, tests automatisés, test d'intégration et preuve de compatibilité requise.
