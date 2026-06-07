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

## Dependency

This project uses [Cobra](https://github.com/spf13/cobra) for the command-line interface.

Documentation:

- Cobra repository: https://github.com/spf13/cobra
- Cobra package docs: https://pkg.go.dev/github.com/spf13/cobra

## Notes

This README is intentionally minimal for now. Add commands, file formats, and usage examples as the application grows.
