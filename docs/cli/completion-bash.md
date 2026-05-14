# `openminutes completion bash`

## Purpose

Generate the bash completion script for OpenMinutes.

The generated script is written to stdout. This command does not require OpenMinutes config or a Feishu/Lark cookie.

## Usage

```sh
openminutes completion bash
```

## Arguments

This command does not accept positional arguments.

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--no-descriptions` | `false` | Disable completion descriptions. |
| `-h, --help` | `false` | Show help for the bash completion command. |

Global flags such as `--config` and `--verbose` are also available.

## Examples

Load completions in the current shell session:

```sh
source <(openminutes completion bash)
```

Install completions for future Linux shell sessions:

```sh
openminutes completion bash > /etc/bash_completion.d/openminutes
```

Install completions for future macOS Homebrew bash sessions:

```sh
openminutes completion bash > "$(brew --prefix)/etc/bash_completion.d/openminutes"
```

Generate without descriptions:

```sh
openminutes completion bash --no-descriptions
```

## Output

The command prints a bash completion script to stdout.

## Errors And Notes

- The generated script depends on the `bash-completion` package.
- Start a new shell after installing the script for future sessions.
- You may need elevated permissions to write to `/etc/bash_completion.d/openminutes`.
