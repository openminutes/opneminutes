# `openminutes delete`

## Purpose

Delete one or more Minutes from the authenticated Feishu/Lark account.

By default, each Minute is moved to trash. With `--destroy`, each Minute is permanently deleted after it is moved to trash. The command always requires `--yes`.

## Usage

```sh
openminutes delete TOKEN... [flags]
```

## Arguments

| Argument | Description |
| --- | --- |
| `TOKEN...` | One or more Minute object tokens. At least one token is required. |

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--yes` | `false` | Confirm deletion without prompting. Required for every delete operation. |
| `--destroy` | `false` | Permanently delete each Minute after moving it to trash. |

Global flags such as `--config` and `--verbose` are also available.

## Examples

```sh
openminutes delete m_abc123 --yes
openminutes delete m_abc123 m_def456 --yes --destroy
```

## Output

Trash deletion prints one line per token:

```txt
Moved m_abc123 to trash
```

Permanent deletion prints one line per token:

```txt
Permanently deleted m_abc123
```

## Errors And Notes

- `--yes` is required. Without it, the command fails before loading config.
- Requires a valid config and cookie after confirmation is provided.
- At least one token is required.
- Blank tokens are rejected.
- When multiple tokens are provided, OpenMinutes processes them in order and stops at the first error.
- `--destroy` is irreversible from the CLI perspective. Confirm the target tokens before running it.
