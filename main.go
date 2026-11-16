package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/urfave/cli/v3"
)

const (
	defaultIconURL = "https://click-collect-pwa-z27s67eciq-ew.a.run.app/icons/favicon-96x96.png"
)

func run(args []string, stdout, stderr io.Writer) error {
	command := &cli.Command{
		Name:      "cote-resto",
		Usage:     "Récupère le menu du jour et l'envoie à Slack.",
		Writer:    stderr,
		ErrWriter: stderr,
		ArgsUsage: "<url-menu> <url-webhook> [<canal-slack>]",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "URL_MENU", Required: true},
			&cli.StringArg{Name: "URL_WEBHOOK", Required: true},
			&cli.StringArg{Name: "CANAL"},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "exclude", Usage: "fragments de noms de stands à exclure, séparés par des virgules"},
			&cli.StringFlag{Name: "username", Usage: "nom Slack personnalisé (vide : nom basé sur la date)"},
			&cli.StringFlag{Name: "icon-url", Usage: "URL de l'icône Slack", Value: defaultIconURL},
			&cli.StringFlag{Name: "timezone", Usage: "fuseau horaire du menu", Value: "Europe/Paris"},
			&cli.DurationFlag{Name: "timeout", Usage: "délai maximal des requêtes HTTP", Value: 10 * time.Second},
			&cli.BoolFlag{Name: "verbose", Usage: "affiche les étapes de récupération et d'envoi"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 0 {
				return errors.New("attendu : URL_MENU URL_WEBHOOK [CANAL]")
			}
			menuURL, webhook := cmd.StringArg("URL_MENU"), cmd.StringArg("URL_WEBHOOK")
			if strings.TrimSpace(menuURL) == "" {
				return errors.New("fournissez l'URL du menu comme premier argument")
			}
			if strings.TrimSpace(webhook) == "" {
				return errors.New("fournissez l'URL du webhook Slack comme deuxième argument")
			}
			if err := validateWebhook(webhook); err != nil {
				return err
			}
			channel := cmd.StringArg("CANAL")
			channelValue, err := normalizeChannel(channel)
			if err != nil {
				return err
			}
			location, err := time.LoadLocation(cmd.String("timezone"))
			if err != nil {
				return fmt.Errorf("fuseau horaire invalide : %s", cmd.String("timezone"))
			}
			duration := cmd.Duration("timeout")
			if duration <= 0 {
				return errors.New("le délai HTTP doit être une durée positive, par exemple 10s")
			}
			client := &http.Client{Timeout: duration}
			date := time.Now().In(location)
			logVerbose := func(format string, args ...interface{}) {
				if cmd.Bool("verbose") {
					fmt.Fprintf(stderr, format+"\n", args...)
				}
			}
			logVerbose("Date du menu : %s (fuseau %s)", date.Format("02/01/2006"), cmd.String("timezone"))
			logVerbose("Récupération et formatage du menu (délai HTTP : %s)…", duration)
			message, err := fetchMenu(client, menuURL, date, strings.Split(cmd.String("exclude"), ","))
			if err != nil {
				return err
			}
			logVerbose("Menu récupéré et formaté.")
			name := cmd.String("username")
			if name == "" {
				name = "Menu du jour — " + date.Format("02/01/2006")
			}
			if channelValue != "" {
				logVerbose("Envoi du menu à Slack (canal demandé : %s)…", channelValue)
			} else {
				logVerbose("Envoi du menu à Slack (canal du webhook)…")
			}
			if err := sendSlackMessage(client, webhook, message, name, cmd.String("icon-url"), channelValue); err != nil {
				return err
			}
			logVerbose("Envoi à Slack terminé.")
			fmt.Fprintln(stdout, "Menu du jour envoyé à Slack.")
			return nil
		},
	}
	return command.Run(context.Background(), append([]string{"cote-resto"}, args...))
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
