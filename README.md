# d2r-farmer

`d2r-farmer` is a web app (HTMX + pure CSS) for tracking Diablo II: Resurrected character gear and rune needs.

## Run

Start the web server:

```bash
go run .
```

By default it listens on `:8080`.

Set a custom address with `ADDR`:

```bash
ADDR=:8090 go run .
```

Then open `http://localhost:8080` (or your custom port).

## Air (Hot Reload)

Install Air:

```bash
go install github.com/air-verse/air@latest
```

Run:

```bash
air
```

## Data Layout

All state is file-based under `data/`:

- `data/chars/*.yaml` for characters and gear
- `data/config.yaml` for provider configuration

## Provider Config

Gear enrichment and URL import use OpenAI and require `data/config.yaml`.

Example:

```yaml
provider: openai
openai:
  api_key: YOUR_OPENAI_API_KEY
  model: gpt-4.1-mini
```

## Features

- Character list + create character form
- Character detail with gear flags (`needed`, `prio`, `known`)
- Add gear with optional weapon-swap
- URL import from guide pages
- Rune need summary with Countess difficulty hints
