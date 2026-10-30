package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseAIContent(t *testing.T) {
	content := "<think>hmm</think>\n```json\n" +
		`{"type":"Feature","slugs":["automate-report-generation","Automate Reports!","automate-report-generation",""]}` +
		"\n```"
	res, err := parseAIContent(content)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != "feature" {
		t.Errorf("type = %q", res.Type)
	}
	want := []string{"automate-report-generation", "automate-reports"}
	if strings.Join(res.Slugs, ",") != strings.Join(want, ",") {
		t.Errorf("slugs = %v, want %v", res.Slugs, want)
	}

	if res, _ := parseAIContent(`{"type":"weird","slugs":["x-y"]}`); res.Type != "" {
		t.Errorf("unknown type must be dropped, got %q", res.Type)
	}
	if _, err := parseAIContent("sorry, no"); err == nil {
		t.Error("expected error for non-JSON content")
	}
}

func TestSuggestFallsBackToNextModel(t *testing.T) {
	var models []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		models = append(models, req.Model)
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth header")
		}
		if req.Model == "busy:free" {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		w.Write([]byte(`{"model":"ok:free","choices":[{"message":{"role":"assistant",
			"content":"{\"type\":\"bugfix\",\"slugs\":[\"fix-cart-payment\"]}"}}]}`))
	}))
	defer srv.Close()

	c := &llmClient{apiKey: "test-key", url: srv.URL, models: []string{"busy:free", "ok:free"}, http: srv.Client()}
	res, err := c.Suggest(context.Background(), "Виправити оплату в кошику")
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != "ok:free" || res.Type != "bugfix" || res.Slugs[0] != "fix-cart-payment" {
		t.Errorf("unexpected result %+v", res)
	}
	if strings.Join(models, ",") != "busy:free,ok:free" {
		t.Errorf("models tried: %v", models)
	}
}

func TestNeedsAI(t *testing.T) {
	cases := map[string]bool{
		"Додати темну тему": false,
		"Fix login":         false,
		"Автоматизація формування звітів для бухгалтерії по філіях": true,
		"one two three four five": true,
	}
	for title, want := range cases {
		if got := needsAI(resolveInput(title, "")); got != want {
			t.Errorf("needsAI(%q) = %v, want %v", title, got, want)
		}
	}
}

func TestBuildAIVariants(t *testing.T) {
	in := resolveInput("Автоматизація формування звітів", "55287")
	v := buildAIVariants(in, aiResult{Slugs: []string{"automate-reports"}})
	if len(v) != 1 || v[0].Value != "feature/55287-automate-reports" {
		t.Errorf("got %+v", v)
	}
	v = buildAIVariants(in, aiResult{Type: "chore", Slugs: []string{"x"}})
	if v[0].Value != "chore/55287-x" {
		t.Errorf("AI type must win, got %+v", v)
	}
}
