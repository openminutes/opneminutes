# `openminutes completion zsh`

## Purpose

Generate the zsh completion script for OpenMinutes.

The generated script is written to stdout. This command does not require OpenMinutes config or a Feishu/Lark cookie.

## Usage

```sh
openminutes completion zsh [flags]
```

## Arguments

This command does not accept positional arguments.

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--no-descriptions` | `false` | Disable completion descriptions. |
| `-h, --help` | `false` | Show help for the zsh completion command. |

Global flags such as `--config` and `--verbose` are also available.

## Examples

Enable zsh completion if it is not already enabled:

```sh
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

Load completions in the current shell session:

```sh
source <(openminutes completion zsh)
```

Install completions for future Linux shell sessions:

```sh
openminutes completion zsh > "${fpath[1]}/_openminutes"
```

Install completions for future macOS Homebrew zsh sessions:

```sh
openminutes completion zsh > "$(brew --prefix)/share/zsh/site-functions/_openminutes"
```

Generate without descriptions:

```sh
openminutes completion zsh --no-descriptions
```

## Output

The command prints a zsh completion script to stdout.

## Errors And Notes

- Start a new shell after installing the script for future sessions.
- The target directory must already be in `fpath` for zsh to discover the completion file.
