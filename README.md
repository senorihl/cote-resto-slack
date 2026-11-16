# Côté Resto

Récupère le menu du jour et l'envoie sur Slack via un webhook.

## Utilisation

```
NAME:
   cote-resto - Récupère le menu du jour et l'envoie à Slack.

USAGE:
   cote-resto [global options] <url-menu> <url-webhook> [<canal-slack>]

GLOBAL OPTIONS:
   --timezone string   fuseau horaire du menu (default: "Europe/Paris")
   --exclude string    fragments de noms de stands à exclure, séparés par des virgules
   --timeout duration  délai maximal des requêtes HTTP (default: 10s)
   --icon-url string   URL de l'icône Slack (default: "https://click-collect-pwa-z27s67eciq-ew.a.run.app/icons/favicon-96x96.png")
   --username string   nom Slack personnalisé (vide : nom basé sur la date)
   --verbose           affiche les étapes de récupération et d'envoi
   --help, -h          show help
```

Exemple :

```sh
./cote-resto 'https://exemple.fr/menu' 'https://hooks.slack.com/services/IDENTIFIANT/IDENTIFIANT/SECRET'
```

Consultez `./cote-resto --help` pour les options. Le fuseau horaire par défaut est `Europe/Paris`. **Le webhook est un secret** : ne le partagez pas et ne le commitez pas.

## Compiler et tester

Avec Go 1.25 ou une version ultérieure :

```sh
go test ./...
go build -o cote-resto .
```

## Docker

```sh
docker build -t cote-resto .
docker run --rm cote-resto URL_MENU URL_WEBHOOK
```

## Releases

Poussez un tag commençant par `v` (par exemple `v1.0.0`) pour déclencher la compilation et la publication des binaires Linux, macOS et Windows dans GitHub Releases.
