# `openminutes completion fish`

## Purpose

Generate the fish completion script for OpenMinutes.

The generated script is written to stdout. This command does not require OpenMinutes config or a Feishu/Lark cookie.

## Usage

```sh
openminutes completion fish [flags]
```

## Arguments

This command does not accept positional arguments.

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--no-descriptions` | `false` | Disable completion descriptions. |
| `-h, --help` | `false` | Show help for the fish completion command. |

Global flags such as `--config` and `--verbose` are also available.

## Examples

Load completions in the current shell session:

```fish
openminutes completion fish | source
```

Install completions for future fish sessions:

```sh
openminutes completion fish > ~/.config/fish/completions/openminutes.fish
```

Generate without descriptions:

```sh
openminutes completion fish --no-descriptions
```

## Output

The command prints a fish completion script to stdout.

## Errors And Notes

- Start a new shell after installing the script for future sessions.
- Create `~/.config/fish/completions` first if it does not exist.
