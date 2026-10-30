package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// settingsForm — екран налаштувань AI (ключ і моделі OpenRouter).
type settingsForm struct {
	key, models textinput.Model
	focus       int // 0 — ключ, 1 — моделі
	err         string
}

func newSettingsForm(cfg Config, width int) settingsForm {
	k := textinput.New()
	k.Prompt = "🔑 "
	k.Placeholder = "sk-or-v1-…  (https://openrouter.ai/keys)"
	k.EchoMode = textinput.EchoPassword
	k.EchoCharacter = '•'
	k.CharLimit = 200
	k.SetValue(cfg.APIKey)

	m := textinput.New()
	m.Prompt = "🤖 "
	m.Placeholder = strings.Join(defaultModels, ", ")
	m.CharLimit = 500
	m.SetValue(cfg.Models)

	w := min(max(width-8, 20), 100)
	k.Width, m.Width = w, w

	f := settingsForm{key: k, models: m}
	f.applyFocus()
	return f
}

func (f *settingsForm) applyFocus() {
	f.key.Blur()
	f.models.Blur()
	if f.focus == 0 {
		f.key.Focus()
	} else {
		f.models.Focus()
	}
}

func (f settingsForm) config() Config {
	return Config{APIKey: strings.TrimSpace(f.key.Value()), Models: strings.TrimSpace(f.models.Value())}
}

// settingsResult повідомляє головній моделі, що робити після натискання клавіші.
type settingsResult int

const (
	settingsStay settingsResult = iota
	settingsSave
	settingsCancel
)

func (f settingsForm) Update(msg tea.Msg) (settingsForm, settingsResult, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			return f, settingsCancel, nil
		case "ctrl+s":
			return f, settingsSave, nil
		case "enter":
			if f.focus == 1 {
				return f, settingsSave, nil
			}
			f.focus = 1
			f.applyFocus()
			return f, settingsStay, nil
		case "tab", "shift+tab", "up", "down":
			f.focus = 1 - f.focus
			f.applyFocus()
			return f, settingsStay, nil
		case "ctrl+t":
			if f.key.EchoMode == textinput.EchoPassword {
				f.key.EchoMode = textinput.EchoNormal
			} else {
				f.key.EchoMode = textinput.EchoPassword
			}
			return f, settingsStay, nil
		}
	}

	var cmd tea.Cmd
	if f.focus == 0 {
		f.key, cmd = f.key.Update(msg)
	} else {
		f.models, cmd = f.models.Update(msg)
	}
	f.err = ""
	return f, settingsStay, cmd
}

func (f settingsForm) View() string {
	field := func(label string, idx int, input textinput.Model) string {
		l, box := labelDimStyle.Render(label), boxStyle
		if f.focus == idx {
			l, box = labelStyle.Render(label), boxActiveStyle
		}
		return l + "\n" + box.Width(input.Width+4).Render(input.View())
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("⚙ Налаштування AI") + "\n\n")
	b.WriteString(field("OpenRouter API-ключ", 0, f.key) + "\n")
	b.WriteString(field("Моделі (через кому, по черзі; порожньо — за замовчуванням)", 1, f.models) + "\n")

	if path, err := configPath(); err == nil {
		b.WriteString(hintStyle.Render("  Зберігається у: "+path) + "\n")
	}
	b.WriteString(hintStyle.Render("  Порожній ключ вимикає AI.") + "\n\n")
	if f.err != "" {
		b.WriteString(errorStyle.Render(f.err) + "\n\n")
	}

	k := func(key, desc string) string { return keyStyle.Render(key) + " " + hintStyle.Render(desc) }
	b.WriteString(strings.Join([]string{k("tab", "поле"), k("enter/ctrl+s", "зберегти"),
		k("ctrl+t", "показати ключ"), k("esc", "скасувати")}, hintStyle.Render(" · ")))
	return b.String()
}
