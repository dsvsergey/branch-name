# branch-name

**English** | [Українська](README.uk.md)

A terminal UI that turns a task title (Cyrillic or Latin) and a task ID into git branch names.
For long titles it asks a free LLM on OpenRouter for short, meaningful English names.

```text
feature/55287-automate-report-generation     ✨ AI
feature/55287-avtomatyzatsiya-formuvannya-zvitiv   (local transliteration)
```

> The interface is in Ukrainian.

## Installation

Requires Go 1.27+.

```sh
go build -o branch-name .

# install globally on macOS/Linux (~/.local/bin must be in PATH)
go build -o ~/.local/bin/branch-name .
# or: go install .   (puts it in $(go env GOPATH)/bin, add that to PATH)

# build for Windows (from any OS)
GOOS=windows GOARCH=amd64 go build -o branch-name.exe .
```

## Usage

```sh
branch-name                                   # interactive
branch-name -id SHOP-15 Виправити помилку в кошику
branch-name "PROJ-123 Додати темну тему"      # the ID is extracted from the title automatically
```

Variants update as you type. The first variant (★) uses a branch type guessed from keywords:
`bugfix` (fix, bug, помилка, виправити…), `hotfix` (urgent, терміново…), `refactor`, `chore`,
and `feature` when nothing matches.

| Flag         | Description                                                   |
|--------------|---------------------------------------------------------------|
| `-id`        | Task ID, e.g. `PROJ-123`, `#42`                               |
| `-model a,b` | OpenRouter model(s) to try in order (overrides settings)      |
| `-no-ai`     | Disable the LLM, use local generation only                    |

### AI variants (OpenRouter)

For long titles (5+ words, or 31+ characters after transliteration) the app asks a free
OpenRouter model for short English names, e.g. `feature/55287-automate-report-generation`.
Short titles are generated locally with no requests. `ctrl+r` (or `r` in the list) forces an
AI request for any title.

Set the API key and models inside the app: `ctrl+s` (or `s` in the list) opens the settings
screen. Get a key at <https://openrouter.ai/keys>. Settings are saved to a file readable only
by its owner (mode 0600):

| OS      | Path                                                   |
|---------|--------------------------------------------------------|
| Windows | `%AppData%\branch-name\config.env`                     |
| macOS   | `~/Library/Application Support/branch-name/config.env` |
| Linux   | `~/.config/branch-name/config.env`                     |

Priority: `-model` flag > settings file > `OPENROUTER_API_KEY` / `OPENROUTER_MODEL` env vars.

- Default models, tried in order: `openrouter/free`, `google/gemma-4-31b-it:free`,
  `nvidia/nemotron-3.5-lightning:free`. If one is rate-limited or fails, the next is used.
- `OPENROUTER_URL` overrides the API endpoint (proxy or compatible server).
- Responses are cached per title for the session, so changing the ID costs no requests.
  The OpenRouter free tier allows about 50 requests per day.

### Keys

| Key                | Action                                       |
|--------------------|----------------------------------------------|
| `tab` / `enter`    | next field                                   |
| `↑↓` (or `jk`)     | select a variant                             |
| `enter` (in list)  | copy and exit                                |
| `c` / `ctrl+y`     | copy without exiting                         |
| `1`–`9`            | quick-copy a variant by number               |
| `g` / `ctrl+g`     | copy as `git checkout -b <branch>`           |
| `r` / `ctrl+r`     | ask AI (even for a short title)              |
| `s` / `ctrl+s`     | settings (API key, models)                   |
| `ctrl+u`           | clear fields                                 |
| `esc` / `q`        | quit                                         |

### Transliteration

Ukrainian uses a simplified readable scheme (`я→ya`, `ю→yu`, `є→ye`, `ї→yi`, `х→kh`,
`щ→shch`); apostrophes are dropped. If the text contains `ы/э/ё/ъ` and none of `і/ї/є/ґ`,
Russian rules apply (`и→i`, `г→g`). The text part is cut to 50 characters at a word boundary,
without trailing filler words like "для" or "for".

## Development

```sh
go test ./...
```

| File          | Purpose                                         |
|---------------|-------------------------------------------------|
| `main.go`     | TUI (Bubble Tea, Lip Gloss)                     |
| `settings.go` | Settings screen                                 |
| `config.go`   | Reading and writing the settings file           |
| `llm.go`      | OpenRouter client with model fallback           |
| `slug.go`     | Transliteration, type detection, variants       |
