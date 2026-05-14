# `openminutes completion`

## Purpose

Generate shell completion scripts for OpenMinutes.

This is the parent command for the shell-specific completion generators. It does not require OpenMinutes config or a Feishu/Lark cookie.

## Usage

```sh
openminutes completion [command]
```

## Commands

| Command | Purpose |
| --- | --- |
| `openminutes completion bash` | Generate the autocompletion script for bash. |
| `openminutes completion zsh` | Generate the autocompletion script for zsh. |
| `openminutes completion fish` | Generate the autocompletion script for fish. |
| `openminutes completion powershell` | Generate the autocompletion script for PowerShell. |

## Flags

| Flag | Description |
| --- | --- |
| `-h, --help` | Show help for the completion command. |

Global flags such as `--config` and `--verbose` are available, but completion generation does not need config.

## Examples

```sh
openminutes completion --help
openminutes completion bash
openminutes completion zsh
openminutes completion fish
openminutes completion powershell
```

## Output

The shell-specific subcommands print completion scripts to stdout.

## Errors And Notes

- Redirect the generated script to the location expected by your shell, or source it in the current shell session.
- Use the shell-specific guide for installation examples.
