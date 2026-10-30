package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

// Безкоштовні моделі на OpenRouter часто змінюються, тому пробуємо по черзі.
// openrouter/free сам обирає доступну безкоштовну модель.
var defaultModels = []string{
	"openrouter/free",
	"google/gemma-4-31b-it:free",
	"nvidia/nemotron-3.5-lightning:free",
}

var branchTypes = map[string]bool{
	"feature": true, "bugfix": true, "hotfix": true, "refactor": true,
	"chore": true, "docs": true, "test": true,
}

const systemPrompt = `You name git branches for a software team.
The user gives a task title (Ukrainian, Russian or English).
Reply with ONLY a JSON object, no markdown, no explanations:
{"type": "<feature|bugfix|hotfix|refactor|chore|docs|test>", "slugs": ["...", "..."]}

Rules for "slugs":
- exactly 5 distinct options, from most to least fitting;
- English, lowercase kebab-case, 2-5 words, at most 40 characters;
- capture WHAT changes (the essence), not a literal word-by-word translation;
- drop filler words (articles, "for", "the", "of", "implement", "task");
- do NOT include the task ID or the type prefix.

Pick "type" from the meaning: bugfix for errors/fixes, hotfix for urgent production fixes,
refactor for restructuring without behaviour change, chore for deps/config/CI, docs, test,
otherwise feature.`

type aiResult struct {
	Type  string
	Slugs []string
	Model string
}

type llmClient struct {
	apiKey string
	url    string
	models []string
	http   *http.Client
}

// newLLMClient повертає nil, якщо ключ не задано — тоді працює лише локальна генерація.
func newLLMClient(cfg Config) *llmClient {
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" {
		return nil
	}
	models := defaultModels
	if list := splitList(cfg.Models); len(list) > 0 {
		models = list
	}
	url := firstNonEmpty(os.Getenv("OPENROUTER_URL"), openRouterURL)
	return &llmClient{apiKey: key, url: url, models: models, http: &http.Client{Timeout: 30 * time.Second}}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Suggest пробує моделі по черзі, доки одна не дасть валідну відповідь.
// ID у запит не передаємо: він не впливає на текстову частину, а кеш тоді працює за назвою.
func (c *llmClient) Suggest(ctx context.Context, title string) (aiResult, error) {
	var errs []error
	for _, model := range c.models {
		res, err := c.call(ctx, model, title)
		if err == nil {
			return res, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", model, err))
		if ctx.Err() != nil {
			break
		}
	}
	return aiResult{}, errors.Join(errs...)
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *llmClient) call(ctx context.Context, model, title string) (aiResult, error) {
	body, _ := json.Marshal(chatRequest{
		Model:       model,
		Messages:    []chatMessage{{"system", systemPrompt}, {"user", "Task title: " + title}},
		Temperature: 0.4,
		MaxTokens:   1500, // з запасом для моделей, що «думають» перед відповіддю
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return aiResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "branch-name")

	resp, err := c.http.Do(req)
	if err != nil {
		return aiResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return aiResult{}, err
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return aiResult{}, fmt.Errorf("HTTP %d: невалідна відповідь", resp.StatusCode)
	}
	if cr.Error != nil {
		return aiResult{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, cr.Error.Message)
	}
	if resp.StatusCode != http.StatusOK || len(cr.Choices) == 0 {
		return aiResult{}, fmt.Errorf("HTTP %d: порожня відповідь", resp.StatusCode)
	}

	res, err := parseAIContent(cr.Choices[0].Message.Content)
	if err != nil {
		return aiResult{}, err
	}
	res.Model = firstNonEmpty(cr.Model, model)
	return res, nil
}

var thinkBlock = regexp.MustCompile(`(?s)<think>.*?</think>`)

// parseAIContent витягує JSON навіть якщо модель обгорнула його в ```json або додала роздуми.
func parseAIContent(content string) (aiResult, error) {
	content = thinkBlock.ReplaceAllString(content, "")
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return aiResult{}, errors.New("у відповіді немає JSON")
	}
	var parsed struct {
		Type  string   `json:"type"`
		Slugs []string `json:"slugs"`
	}
	if err := json.Unmarshal([]byte(content[start:end+1]), &parsed); err != nil {
		return aiResult{}, fmt.Errorf("невалідний JSON: %w", err)
	}

	res := aiResult{Type: strings.ToLower(strings.TrimSpace(parsed.Type))}
	if !branchTypes[res.Type] {
		res.Type = ""
	}
	seen := map[string]bool{}
	for _, s := range parsed.Slugs {
		// slugify гарантує, що в гілку потрапить лише безпечний для git текст
		if s = slugify(s, maxSlugLen); s != "" && !seen[s] {
			seen[s] = true
			res.Slugs = append(res.Slugs, s)
		}
	}
	if len(res.Slugs) == 0 {
		return aiResult{}, errors.New("модель не повернула жодного варіанта")
	}
	return res, nil
}
