# `openminutes`

## Purpose

`openminutes` is the root command for the OpenMinutes CLI. It provides global flags, version output, command discovery, and shell completion entry points.

Commands that call Feishu/Lark APIs load configuration and require a valid authentication cookie. The root help and completion commands do not require configuration.

## Usage

```sh
openminutes [command]
```

## Commands

| Command | Purpose |
| --- | --- |
| `openminutes list` | List Minutes from the current account. |
| `openminutes get TOKEN` | Export text from a Minute. |
| `openminutes upload FILE` | Upload media to Minutes for transcription. |
| `openminutes delete TOKEN...` | Delete one or more Minutes from the current account. |
| `openminutes completion` | Generate shell completion scripts. |
| `openminutes help [command]` | Show help for a command. |

## Global Flags

| Flag | Description |
| --- | --- |
| `--config string` | Config file path. The help default is `~/.config/openminutes/config.toml`. |
| `--verbose` | Enable verbose debug logging on stderr. |
| `-h, --help` | Show help. |
| `-v, --version` | Print the version and exit. |

## Examples

```sh
openminutes --help
openminutes --version
openminutes --config ./config.toml list
openminutes --verbose list --json
```

## Output

`openminutes --help` prints available commands and global flags.

`openminutes --version` prints:

```txt
openminutes version <version>
```

Development builds print `dev` as the version.

## Errors And Notes

- There is no standalone `openminutes version` command. Version output is exposed through `-v, --version`.
- Config-backed commands validate `base_url`, `space_base_url`, and `cookie` before calling Feishu/Lark APIs.
- Use `--verbose` when debugging config loading, validation, or API calls.
