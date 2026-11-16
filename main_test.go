package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const fixture = `<!doctype html><script>var loadingData = [[{"pole":"L' Atelier","dateUs":20260928,"nom":"Tajine de poulet","info1":"Au citron confit","prix":"4,85 €"},{"pole":"Le Grillé","dateUs":"20260928","nom":"<!channel>","rupture":"TRUE"},{"pole":"L' Atelier","dateUs":20260928,"nom":"Semoule BIO","info1":"Légumes au bouillon","accompagnement":"TRUE"},{"pole":"Cafet","dateUs":20260928,"nom":"Café"},{"pole":"Le Grillé","dateUs":20260929,"nom":"Demain"}]];</script>`

func TestMenuFormatting(t *testing.T) {
	groups, err := extractLoadingData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	location, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 9, 27, 22, 30, 0, 0, time.UTC).In(location)
	message, err := formatMenu(groups, date, []string{"cafet"})
	if err != nil {
		t.Fatal(err)
	}
	want := "*L' Atelier*\n• *Tajine de poulet* — Au citron confit — 4,85 €\n\n*Le Grillé*\n• *&lt;!channel&gt;* _(indisponible)_\n\n*Accompagnements*\n• *Semoule BIO*\n• *Légumes au bouillon*"
	if message != want {
		t.Fatalf("got:\n%s\nwant:\n%s", message, want)
	}
}

func TestExtractInvalidData(t *testing.T) {
	for _, input := range []string{"no data", "var loadingData = {}", "var loadingData = [[", "var loadingData = [[{bad}]]", `var loadingData = [[{"pole":"A","nom":"X","dateUs":20260928,"info1":42}]]`, `var loadingData = [[{"pole":"A","nom":"X","dateUs":null}]]`, `var loadingData = [null]`} {
		t.Run(input, func(t *testing.T) {
			if _, err := extractLoadingData(input); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	groups, err := extractLoadingData(`var loadingData = [[{"pole":"A","nom":"X ] } \\\" [","dateUs":20260928}]];`)
	if err != nil || len(groups) != 1 {
		t.Fatalf("escaped JSON: %v", err)
	}
}

func TestMissingMenuAndLimit(t *testing.T) {
	groups, err := extractLoadingData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := formatMenu(groups, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), nil); err == nil {
		t.Fatal("expected missing menu error")
	}
	groups[0][0].Nom = strings.Repeat("😀", slackMessageLimit)
	if _, err := formatMenu(groups, time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), nil); err == nil {
		t.Fatal("expected length error")
	}
}

func TestFetchMenuHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	_, err := fetchMenu(server.Client(), server.URL, time.Now(), nil)
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSlackWebhook(t *testing.T) {
	var payload map[string]string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	if err := sendSlackMessage(server.Client(), server.URL, "menu", "Menu du jour — 28/09/2026", "https://example.org/icon.png", "#test"); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"text": "menu", "username": "Menu du jour — 28/09/2026", "icon_url": "https://example.org/icon.png", "channel": "#test"} {
		if payload[key] != want {
			t.Errorf("%s = %q, want %q", key, payload[key], want)
		}
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("failure at %s", r.URL)
}

func TestSlackErrorsHideWebhook(t *testing.T) {
	secret := "https://example.org/services/private-secret"
	client := &http.Client{Transport: failingTransport{}}
	err := sendSlackMessage(client, secret, "menu", "name", "icon", "")
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "private-secret") {
		t.Fatalf("unsafe error: %v", err)
	}
	if err := validateWebhook("http://example.org"); err == nil {
		t.Fatal("HTTP webhook accepted")
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer server.Close()
	if err := sendSlackMessage(server.Client(), server.URL, "menu", "name", "icon", ""); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChannel(t *testing.T) {
	for input, want := range map[string]string{"": "", "test": "#test", "#test": "#test"} {
		got, err := normalizeChannel(input)
		if err != nil || got != want {
			t.Fatalf("%q: %q %v", input, got, err)
		}
	}
	if _, err := normalizeChannel("bad channel"); err == nil {
		t.Fatal("invalid channel accepted")
	}
}

func TestCLI(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "https://example.org/ignored")
	t.Setenv("MENU_URL", "https://example.org/ignored")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "timezone") || !strings.Contains(stderr.String(), "--help") {
		t.Fatal("missing help")
	}
	if err := run([]string{"preview"}, &stdout, &stderr); err == nil {
		t.Fatal("unexpected positional argument accepted")
	}
	if err := run(nil, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "URL_MENU") {
		t.Fatalf("missing menu URL accepted: %v", err)
	}
	if err := run([]string{"https://example.org/menu"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "URL_WEBHOOK") {
		t.Fatalf("missing webhook accepted: %v", err)
	}
	if err := run([]string{"https://example.org/menu", "http://example.org/secret"}, &stdout, &stderr); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("invalid webhook accepted or exposed: %v", err)
	}
	if err := run([]string{"https://example.org/menu", "https://example.org/hook", "test", "extra"}, &stdout, &stderr); err == nil {
		t.Fatal("extra argument accepted")
	}
	if err := run([]string{"--menu-url", "https://example.org/menu"}, &stdout, &stderr); err == nil {
		t.Fatal("obsolete option accepted")
	}
}

func TestCLISendsTodaysMenu(t *testing.T) {
	location, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(location).Format("20060102")
	menuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `var loadingData = [[{"pole":"Stand","nom":"Plat","dateUs":%s}]];`, today)
	}))
	defer menuServer.Close()
	// run uses the system HTTP client, so use an intercepted transport for the
	// HTTPS webhook rather than making any real Slack request.
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()
	var payload map[string]string
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			return &http.Response{StatusCode: 200, Body: http.NoBody, Header: make(http.Header), Request: r}, nil
		}
		return originalTransport.RoundTrip(r)
	})
	t.Setenv("SLACK_CHANNEL", "ignored")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--verbose", "--username", "Cantine", menuServer.URL, "https://example.org/private", "test"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if payload["text"] != "*Stand*\n• *Plat*" || payload["username"] != "Cantine" || payload["channel"] != "#test" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	logs := stderr.String()
	for _, step := range []string{"Date du menu", "Récupération", "Menu récupéré", "canal demandé : #test", "Envoi à Slack terminé"} {
		if !strings.Contains(logs, step) {
			t.Errorf("missing verbose step %q in %q", step, logs)
		}
	}
	if strings.Contains(logs, "https://example.org/private") || strings.Contains(logs, menuServer.URL) {
		t.Fatal("verbose output exposes an URL")
	}
	payload = nil
	stderr.Reset()
	if err := run([]string{menuServer.URL, "https://example.org/private"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected output without verbose: %s", stderr.String())
	}
	if _, ok := payload["channel"]; ok {
		t.Fatalf("unexpected channel without option: %#v", payload)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
