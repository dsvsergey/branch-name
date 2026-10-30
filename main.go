package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type focus int

const (
	focusTitle focus = iota
	focusID
	focusList
	focusCount
)

var (
	accent  = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#A78BFA"}
	muted   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#6B7280"}
	success = lipgloss.AdaptiveColor{Light: "#059669", Dark: "#34D399"}
	danger  = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F87171"}

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).
			Background(accent).Padding(0, 1)
	labelStyle     = lipgloss.NewStyle().Bold(true)
	labelDimStyle  = lipgloss.NewStyle().Foreground(muted)
	hintStyle      = lipgloss.NewStyle().Foreground(muted).Italic(true)
	boxStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(0, 1)
	boxActiveStyle = boxStyle.BorderForeground(accent)
	selectedStyle  = lipgloss.NewStyle().Bold(true).Foreground(accent)
	tagStyle       = lipgloss.NewStyle().Foreground(muted)
	successStyle   = lipgloss.NewStyle().Foreground(success).Bold(true)
	errorStyle     = lipgloss.NewStyle().Foreground(danger).Bold(true)
	keyStyle       = lipgloss.NewStyle().Foreground(accent).Bold(true)
)

// aiDoneMsg — відповідь LLM для конкретної назви задачі.
type aiDoneMsg struct {
	title string
	res   aiResult
	err   error
}

type model struct {
	title, id textinput.Model
	focus     focus
	in        Input
	variants  []variant
	aiCount   int // скільки перших варіантів прийшли від AI
	cursor    int
	gitCmd    bool // копіювати як `git checkout -b ...`
	status    string
	statusErr bool
	width     int
	result    string // друкується після виходу

	cfg      Config
	settings *settingsForm // не nil, поки відкрито екран налаштувань

	ai         *llmClient
	aiCache    map[string]aiResult // назва -> відповідь, щоб не витрачати ліміт повторно
	aiLoading  string              // назва, для якої зараз іде запит
	aiErr      string
	spinner    spinner.Model
	startupCmd tea.Cmd
}

func newModel(title, id string, cfg Config, ai *llmClient) model {
	t := textinput.New()
	t.Placeholder = "Вставте назву задачі (кирилиця чи латиниця)…"
	t.Prompt = "✎ "
	t.CharLimit = 500
	t.SetValue(title)

	i := textinput.New()
	i.Placeholder = "PROJ-123, #42 або 42 (можна лишити порожнім)"
	i.Prompt = "# "
	i.CharLimit = 50
	i.SetValue(id)

	m := model{
		title: t, id: i, width: 80,
		cfg: cfg, ai: ai, aiCache: map[string]aiResult{},
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(selectedStyle)),
	}
	if title != "" && id != "" {
		m.focus = focusList
	} else if title != "" {
		m.focus = focusID
	}
	m.applyFocus()
	m.recompute()
	if title != "" {
		m.startupCmd = m.maybeAI(false)
	}
	return m
}

func (m *model) applyFocus() {
	m.title.Blur()
	m.id.Blur()
	switch m.focus {
	case focusTitle:
		m.title.Focus()
	case focusID:
		m.id.Focus()
	}
}

func (m *model) recompute() {
	m.in = resolveInput(m.title.Value(), m.id.Value())
	var ai []variant
	if res, ok := m.aiCache[m.in.Title]; ok {
		ai = buildAIVariants(m.in, res)
	}
	m.aiCount = len(ai)
	m.variants = append(ai, buildVariants(m.in)...)
	if m.cursor >= len(m.variants) {
		m.cursor = max(0, len(m.variants)-1)
	}
}

// maybeAI запускає запит до LLM: автоматично — лише для довгих назв, з force — для будь-яких.
func (m *model) maybeAI(force bool) tea.Cmd {
	title := m.in.Title
	if title == "" {
		return nil
	}
	if m.ai == nil {
		if force {
			m.status, m.statusErr = "AI вимкнено: задайте змінну OPENROUTER_API_KEY", true
		}
		return nil
	}
	if !force && !needsAI(m.in) {
		return nil
	}
	if _, cached := m.aiCache[title]; cached && !force {
		return nil
	}
	if m.aiLoading == title {
		return nil
	}
	m.aiLoading, m.aiErr = title, ""
	ai := m.ai
	request := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		res, err := ai.Suggest(ctx, title)
		return aiDoneMsg{title, res, err}
	}
	return tea.Batch(m.spinner.Tick, request)
}

func (m model) selected() string {
	if len(m.variants) == 0 {
		return ""
	}
	v := m.variants[m.cursor].Value
	if m.gitCmd {
		return "git checkout -b " + v
	}
	return v
}

func (m *model) copySelected() bool {
	s := m.selected()
	if s == "" {
		m.status, m.statusErr = "Немає що копіювати — введіть назву або ID", true
		return false
	}
	if err := clipboard.WriteAll(s); err != nil {
		m.status, m.statusErr = "Не вдалося скопіювати: "+err.Error(), true
		return false
	}
	m.status, m.statusErr = "✓ Скопійовано: "+s, false
	return true
}

func (m model) Init() tea.Cmd { return tea.Batch(textinput.Blink, m.startupCmd) }

// moveFocus перемикає поле; вихід із поля назви — природний момент запитати AI.
func (m *model) moveFocus(to focus) tea.Cmd {
	from := m.focus
	m.focus = to
	m.applyFocus()
	if from == focusTitle || to == focusList {
		return m.maybeAI(false)
	}
	return nil
}

func (m *model) openSettings() {
	f := newSettingsForm(m.cfg, m.width)
	m.settings = &f
}

func (m model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	f, res, cmd := m.settings.Update(msg)
	m.settings = &f
	switch res {
	case settingsCancel:
		m.settings = nil
	case settingsSave:
		cfg := f.config()
		if _, err := saveConfigFile(cfg); err != nil {
			m.settings.err = "Не вдалося зберегти: " + err.Error()
			return m, nil
		}
		// нові ключ/модель — старі відповіді та помилки більше не актуальні
		m.cfg, m.ai, m.settings = cfg, newLLMClient(cfg), nil
		m.aiCache, m.aiLoading, m.aiErr = map[string]aiResult{}, "", ""
		m.status, m.statusErr = "✓ Налаштування збережено", false
		if m.ai == nil {
			m.status = "✓ Налаштування збережено (AI вимкнено — ключ порожній)"
		}
		m.recompute()
		return m, m.maybeAI(false)
	}
	return m, cmd
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.settings != nil {
		switch msg.(type) {
		case tea.WindowSizeMsg, spinner.TickMsg, aiDoneMsg:
			// обробляються нижче як завжди
		default:
			return m.updateSettings(msg)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		w := min(max(m.width-8, 20), 100)
		m.title.Width, m.id.Width = w, w
		return m, nil

	case spinner.TickMsg:
		if m.aiLoading == "" {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case aiDoneMsg:
		if msg.err == nil {
			m.aiCache[msg.title] = msg.res
		}
		if msg.title == m.aiLoading {
			m.aiLoading = ""
			if msg.err != nil {
				m.aiErr = "AI недоступний: " + strings.ReplaceAll(msg.err.Error(), "\n", " · ")
				if w := max(m.width-10, 40); len([]rune(m.aiErr)) > w {
					m.aiErr = string([]rune(m.aiErr)[:w-1]) + "…"
				}
			}
		}
		if msg.title == m.in.Title && msg.err == nil {
			m.cursor = 0
		}
		m.recompute()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "tab":
			return m, m.moveFocus((m.focus + 1) % focusCount)
		case "shift+tab":
			return m, m.moveFocus((m.focus + focusCount - 1) % focusCount)
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "ctrl+n":
			if m.cursor < len(m.variants)-1 {
				m.cursor++
			}
			return m, nil
		case "ctrl+y":
			m.copySelected()
			return m, nil
		case "ctrl+g":
			m.gitCmd = !m.gitCmd
			return m, nil
		case "ctrl+r":
			return m, m.maybeAI(true)
		case "ctrl+s":
			m.openSettings()
			return m, textinput.Blink
		case "ctrl+u":
			m.title.SetValue("")
			m.id.SetValue("")
			m.focus, m.status, m.aiErr = focusTitle, "", ""
			m.applyFocus()
			m.recompute()
			return m, nil
		case "enter":
			if m.focus != focusList {
				return m, m.moveFocus(m.focus + 1)
			}
			if m.copySelected() {
				m.result = m.selected()
				return m, tea.Quit
			}
			return m, nil
		}

		if m.focus == focusList {
			switch k := msg.String(); k {
			case "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "j":
				if m.cursor < len(m.variants)-1 {
					m.cursor++
				}
			case "c", "y":
				m.copySelected()
			case "g":
				m.gitCmd = !m.gitCmd
			case "r":
				return m, m.maybeAI(true)
			case "s":
				m.openSettings()
				return m, textinput.Blink
			case "e", "i":
				m.focus = focusTitle
				m.applyFocus()
			case "q":
				return m, tea.Quit
			default:
				if n, err := strconv.Atoi(k); err == nil && n >= 1 && n <= len(m.variants) {
					m.cursor = n - 1
					m.copySelected()
				}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	if m.focus == focusTitle {
		m.title, cmd = m.title.Update(msg)
	} else if m.focus == focusID {
		m.id, cmd = m.id.Update(msg)
	}
	m.recompute()
	m.status = ""
	return m, cmd
}

func (m model) field(label string, f focus, input textinput.Model) string {
	l, box := labelDimStyle.Render(label), boxStyle
	if m.focus == f {
		l, box = labelStyle.Render(label), boxActiveStyle
	}
	return l + "\n" + box.Width(input.Width+4).Render(input.View())
}

// aiLine описує стан AI для поточної назви.
func (m model) aiLine() string {
	switch {
	case m.in.Title == "":
		return ""
	case m.aiLoading == m.in.Title:
		return m.spinner.View() + hintStyle.Render(" AI підбирає назви…")
	case m.aiErr != "" && m.aiCount == 0:
		return errorStyle.Render(m.aiErr)
	case m.aiCount > 0:
		return hintStyle.Render("✨ " + m.aiCache[m.in.Title].Model)
	case m.ai == nil && needsAI(m.in):
		return hintStyle.Render("Довга назва — додайте API-ключ у налаштуваннях (ctrl+s), щоб отримати AI-варіанти")
	case m.ai != nil && needsAI(m.in):
		return hintStyle.Render("Довга назва — AI запуститься після переходу до списку (або ctrl+r)")
	case m.ai != nil:
		return hintStyle.Render("Коротка назва — генеруємо локально (ctrl+r — запитати AI)")
	}
	return ""
}

func (m model) View() string {
	if m.settings != nil {
		return lipgloss.NewStyle().Padding(1, 2).Render(m.settings.View())
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("🌿 Branch Name Generator") + "\n\n")
	b.WriteString(m.field("Назва задачі", focusTitle, m.title) + "\n")
	b.WriteString(m.field("ID задачі", focusID, m.id) + "\n")

	if m.in.IDExtracted {
		b.WriteString(hintStyle.Render("  ID знайдено в назві: "+m.in.ID) + "\n")
	}
	b.WriteString("\n")

	header := "Варіанти"
	if m.gitCmd {
		header += tagStyle.Render("  (копіюється як git checkout -b …)")
	}
	if m.focus == focusList {
		b.WriteString(labelStyle.Render(header) + "\n")
	} else {
		b.WriteString(labelDimStyle.Render(header) + "\n")
	}
	if line := m.aiLine(); line != "" {
		b.WriteString("  " + line + "\n")
	}

	if len(m.variants) == 0 {
		b.WriteString(hintStyle.Render("  Почніть вводити назву — варіанти з'являться тут") + "\n")
	}
	valueW := 0
	for _, v := range m.variants {
		valueW = max(valueW, len(v.Value))
	}
	for i, v := range m.variants {
		if i == m.aiCount && m.aiCount > 0 {
			b.WriteString(tagStyle.Render("   ── локально ──") + "\n")
		}
		num := fmt.Sprintf("%2d", i+1)
		value := fmt.Sprintf("%-*s", valueW, v.Value)
		tag := tagStyle.Render(v.Label)
		if i == m.cursor {
			b.WriteString(selectedStyle.Render(" ▸"+num+"  "+value) + "  " + tag + "\n")
		} else {
			b.WriteString("  " + tagStyle.Render(num) + "  " + value + "  " + tag + "\n")
		}
	}

	b.WriteString("\n")
	if m.status != "" {
		if m.statusErr {
			b.WriteString(errorStyle.Render(m.status) + "\n\n")
		} else {
			b.WriteString(successStyle.Render(m.status) + "\n\n")
		}
	}
	b.WriteString(m.help())
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m model) help() string {
	k := func(key, desc string) string { return keyStyle.Render(key) + " " + hintStyle.Render(desc) }
	sep := hintStyle.Render(" · ")
	var items []string
	if m.focus == focusList {
		items = []string{k("↑↓/jk", "вибір"), k("enter", "копіювати і вийти"), k("c", "копіювати"),
			k("1-9", "швидко скопіювати"), k("r", "AI"), k("s", "налаштування"), k("g", "git checkout"), k("e", "редагувати"), k("q", "вихід")}
	} else {
		items = []string{k("tab", "далі"), k("enter", "далі"), k("↑↓", "вибір"), k("ctrl+y", "копіювати"),
			k("ctrl+r", "AI"), k("ctrl+s", "налаштування"), k("ctrl+g", "git checkout"), k("ctrl+u", "очистити"), k("esc", "вихід")}
	}
	return strings.Join(items, sep)
}

func main() {
	id := flag.String("id", "", "ID задачі (напр. PROJ-123)")
	modelName := flag.String("model", "", "модель(і) OpenRouter через кому (або змінна OPENROUTER_MODEL)")
	noAI := flag.Bool("no-ai", false, "не використовувати LLM")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Використання: branch-name [-id PROJ-123] [-model ...] [-no-ai] [назва задачі]")
		flag.PrintDefaults()
	}
	flag.Parse()
	title := strings.Join(flag.Args(), " ")

	cfg := effectiveConfig(*modelName)
	var ai *llmClient
	if !*noAI {
		ai = newLLMClient(cfg)
	}

	p := tea.NewProgram(newModel(title, *id, cfg, ai), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Помилка:", err)
		os.Exit(1)
	}
	if r := final.(model).result; r != "" {
		fmt.Println(successStyle.Render("✓ Скопійовано в буфер:"), r)
	}
}
