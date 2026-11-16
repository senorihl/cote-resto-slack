# Côté Resto

Récupère le menu du jour et l'envoie sur Slack via un webhook.

## Utilisation

```sh
./cote-resto [options] URL_MENU URL_WEBHOOK [CANAL]
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
