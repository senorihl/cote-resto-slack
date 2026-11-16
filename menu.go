package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

const slackMessageLimit = 40000

type menuItem struct {
	Pole           string
	DateUs         json.RawMessage
	Nom            string
	Info1          string
	Info2          string
	Prix           string
	Accompagnement string
	Rupture        string
}

var loadingDataAssignment = regexp.MustCompile(`\bvar\s+loadingData\s*=\s*`)

func extractLoadingData(page string) ([][]menuItem, error) {
	loc := loadingDataAssignment.FindStringIndex(page)
	if loc == nil {
		return nil, errors.New("données du menu introuvables dans la page source")
	}
	data := strings.TrimLeft(page[loc[1]:], " \t\r\n")
	if !strings.HasPrefix(data, "[") {
		return nil, errors.New("format des données du menu invalide")
	}
	depth, end := 0, -1
	inString, escaped := false, false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inString {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil, errors.New("données du menu incomplètes")
	}
	var groups [][]map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data[:end]), &groups); err != nil {
		return nil, errors.New("données du menu invalides ou structure inattendue")
	}
	result := make([][]menuItem, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			return nil, errors.New("structure des données du menu inattendue")
		}
		items := make([]menuItem, 0, len(group))
		for _, raw := range group {
			var item menuItem
			fields := map[string]*string{"pole": &item.Pole, "nom": &item.Nom, "info1": &item.Info1, "info2": &item.Info2, "prix": &item.Prix, "accompagnement": &item.Accompagnement, "rupture": &item.Rupture}
			for name, target := range fields {
				value, ok := raw[name]
				if !ok {
					if name == "pole" || name == "nom" {
						return nil, errors.New("structure des données du menu inattendue")
					}
					continue
				}
				if string(value) == "null" || json.Unmarshal(value, target) != nil {
					return nil, errors.New("structure des données du menu inattendue")
				}
			}
			item.DateUs = raw["dateUs"]
			var dateString string
			var dateNumber json.Number
			if len(item.DateUs) == 0 || string(item.DateUs) == "null" || (json.Unmarshal(item.DateUs, &dateString) != nil && json.Unmarshal(item.DateUs, &dateNumber) != nil) {
				return nil, errors.New("structure des données du menu inattendue")
			}
			items = append(items, item)
		}
		result = append(result, items)
	}
	return result, nil
}

func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func formatMenu(groups [][]menuItem, date time.Time, excluded []string) (string, error) {
	today, _ := strconv.ParseFloat(date.Format("20060102"), 64)
	byPole := make(map[string][]menuItem)
	var poles []string
	for _, group := range groups {
		for _, item := range group {
			var dateValue string
			if json.Unmarshal(item.DateUs, &dateValue) != nil {
				dateValue = string(item.DateUs)
			}
			itemDate, err := strconv.ParseFloat(strings.TrimSpace(dateValue), 64)
			if err != nil || itemDate != today || strings.TrimSpace(item.Nom) == "" {
				continue
			}
			ignored := false
			for _, fragment := range excluded {
				fragment = strings.TrimSpace(fragment)
				if fragment != "" && strings.Contains(strings.ToLower(item.Pole), strings.ToLower(fragment)) {
					ignored = true
					break
				}
			}
			if ignored {
				continue
			}
			pole := strings.TrimSpace(item.Pole)
			if _, ok := byPole[pole]; !ok {
				poles = append(poles, pole)
			}
			byPole[pole] = append(byPole[pole], item)
		}
	}
	if len(poles) == 0 {
		return "", errors.New("aucun menu disponible pour aujourd'hui")
	}
	var lines, accompaniments []string
	for _, pole := range poles {
		lines = append(lines, "", "*"+slackEscape(pole)+"*")
		for _, item := range byPole[pole] {
			if strings.EqualFold(item.Accompagnement, "TRUE") {
				for _, choice := range []string{item.Nom, item.Info1, item.Info2} {
					if choice = strings.TrimSpace(choice); choice != "" {
						accompaniments = append(accompaniments, "• *"+slackEscape(choice)+"*")
					}
				}
				continue
			}
			var descriptions []string
			for _, value := range []string{item.Info1, item.Info2} {
				if value = strings.TrimSpace(value); value != "" {
					descriptions = append(descriptions, slackEscape(value))
				}
			}
			var details []string
			if len(descriptions) > 0 {
				details = append(details, strings.Join(descriptions, " · "))
			}
			if price := strings.TrimSpace(item.Prix); price != "" {
				details = append(details, slackEscape(price))
			}
			line := "• *" + slackEscape(strings.TrimSpace(item.Nom)) + "*"
			if len(details) > 0 {
				line += " — " + strings.Join(details, " — ")
			}
			if strings.EqualFold(item.Rupture, "TRUE") {
				line += " _(indisponible)_"
			}
			lines = append(lines, line)
		}
	}
	if len(accompaniments) > 0 {
		lines = append(lines, "", "*Accompagnements*")
		lines = append(lines, accompaniments...)
	}
	message := strings.TrimPrefix(strings.Join(lines, "\n"), "\n")
	// The source CLI checks the full formatted message, including its date title.
	fullMessage := "*Menu du jour — " + date.Format("02/01/2006") + "*\n\n" + message
	if len(utf16.Encode([]rune(fullMessage))) > slackMessageLimit {
		return "", fmt.Errorf("le menu dépasse la limite Slack de %d caractères", slackMessageLimit)
	}
	return message, nil
}

func fetchMenu(client *http.Client, menuURL string, date time.Time, excluded []string) (string, error) {
	response, err := client.Get(menuURL)
	if err != nil {
		return "", errors.New("impossible de joindre le service de menu ; vérifiez la connexion et l'URL")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("le service de menu a répondu HTTP %d", response.StatusCode)
	}
	page, err := io.ReadAll(response.Body)
	if err != nil {
		return "", errors.New("impossible de lire la réponse du service de menu")
	}
	groups, err := extractLoadingData(string(page))
	if err != nil {
		return "", err
	}
	return formatMenu(groups, date, excluded)
}
