package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

func validateWebhook(webhook string) error {
	url, err := url.Parse(webhook)
	if err != nil || url.Scheme != "https" || url.Host == "" || url.User != nil {
		return errors.New("l'URL du webhook Slack doit être une URL HTTPS valide")
	}
	return nil
}

var channelPattern = regexp.MustCompile(`^#[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func normalizeChannel(channel string) (string, error) {
	if channel == "" {
		return "", nil
	}
	if !strings.HasPrefix(channel, "#") {
		channel = "#" + channel
	}
	if !channelPattern.MatchString(channel) {
		return "", errors.New("nom de canal Slack invalide")
	}
	return channel, nil
}

func sendSlackMessage(client *http.Client, webhook, message, username, iconURL, channel string) error {
	if err := validateWebhook(webhook); err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		Text     string `json:"text"`
		Username string `json:"username"`
		IconURL  string `json:"icon_url"`
		Channel  string `json:"channel,omitempty"`
	}{message, username, iconURL, channel})
	if err != nil {
		return errors.New("impossible de préparer le message Slack")
	}
	response, err := client.Post(webhook, "application/json", bytes.NewReader(payload))
	if err != nil {
		return errors.New("impossible de joindre Slack ; vérifiez le webhook et la connexion")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Slack a refusé le message (HTTP %d)", response.StatusCode)
	}
	return nil
}
