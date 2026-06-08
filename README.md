# d2r-farmer

`d2r-farmer` is a simple command-line tool for tracking gear and other items you want for a Diablo II: Resurrected character.

The workflow is intentionally lightweight:

1. Download or copy an item list from a source such as Maxroll.
2. Add the items you want manually.
3. Track what you already have.
4. Store everything in YAML files under the `data/` directory.

There is no database. The project is designed to stay file-based so the data remains easy to inspect, edit, and back up.

## Data

The application keeps its state in YAML files in `data/`. The exact file layout can evolve as the project grows, but the goal is to keep all user-managed data in that directory.

## Build

Build the CLI from the repository root with:

```bash
go build -o bin/d2r-farmer .
```

## Help

Show the top-level help with:

```bash
go run . --help
```

If you build the binary first, you can also run:

```bash
./bin/d2r-farmer --help
```

## Initialize Provider

Initialize the LLM provider setup (currently OpenAI only):

```bash
go run . init --provider openai --api-key "$OPENAI_API_KEY"
```

Optional model override:

```bash
go run . init --provider openai --api-key "$OPENAI_API_KEY" --model gpt-4.1-mini
```

This writes `data/config.yaml`.

## Add Character

Create a new character file:

```bash
go run . char "Nova Sorc" --class sorceress
```

Add mandatory requirements while creating the character:

```bash
go run . char fury --class druid --mandatory "cannot be frozen"
```

If provider config is available, character creation also enriches relevant breakpoints (for example FHR/FCR/FBR/IAS with class/form context such as druid werewolf vs human) and stores them under `requirements.breakpoints`.

This creates `data/chars/nova-sorc.yaml`.

## Add Gear

Resolve and append gear to a character through OpenAI structured output:

```bash
go run . gear fury "breath of fury"
```

Force a slot when needed (for example class-specific head runewords):

```bash
go run . gear fury wisdom --head
```

Available slot flags: `--weapon`, `--head`, `--armor`, `--belt`, `--ring`, `--amulet`, `--inventory`.

Mark gear as weapon-swap:

```bash
go run . gear fury "call to arms" --weapon-swap
go run . gear fury "monarch spirit" --weapon-swap
go run . gear fury "harmony bow" --weapon-swap
```

When `--weapon-swap` is set, the tool asks the LLM to infer weapon swap role details (such as main/offhand).

This updates `data/chars/fury.yaml` and appends a structured entry under `gear` from LLM output.

Expected fields include:

- exact_name
- slot
- kind
- runes
- possible_bases
- best_in_slot_base
- notes
- sources

## List

List all characters:

```bash
go run . list
```

## Info

Show character details including class, mandatory requirements, and gear flags (`needed`, `prio`, `known`):

```bash
go run . info fury
```

`info` includes the stored breakpoint summary and notes before the gear sections.

`info` also prints item kind (`unique`, `set`, `runeword`, ...), and for runewords it includes rune order, best base, and possible bases.

Gear is shown grouped by slot categories: `weapon`, `head`, `armor`, `belt`, `ring`, `amulet`, `inventory`.

## Found

Mark a tracked gear item as found:

```bash
go run . found fury "harmony bow"
```

## Remove

Remove a tracked gear item by its number from `info` output:

```bash
go run . remove fury 1
```

Or remove by slot-specific index:

```bash
go run . remove fury 1 --slot ring
```

## Bases

Interactive update of runeword bases:

```bash
go run . bases fury
```

This lists runewords, asks for a number, then prompts for a comma-separated base list. The first base becomes best-in-slot, and any non-listed previous bases are removed.
It also supports a prioritized best-in-slot list (multiple bases, ordered by priority).

Non-interactive update by item number is still available:

```bash
go run . bases fury 1 --set "Ethereal Berserker Axe, Ethereal Archon Staff" --best "Ethereal Archon Staff, Ethereal Berserker Axe"
```

## Runes

List all runes still needed for not-yet-found gear:

```bash
go run . runes fury
```

## List Models

List available models for a provider:

```bash
go run . list-models openai
```

## Import

Import gear from a Maxroll guide URL (gear section only):

```bash
go run . import fury "https://maxroll.gg/d2/guides/werewolf-fury-druid"
```


## Dependency

This project uses [Cobra](https://github.com/spf13/cobra) for the command-line interface.

Documentation:

- Cobra repository: https://github.com/spf13/cobra
- Cobra package docs: https://pkg.go.dev/github.com/spf13/cobra

## Notes

This README is intentionally minimal for now. Add commands, file formats, and usage examples as the application grows.
