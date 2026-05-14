# Quick Start

This guide shows how to build OpenMinutes from source, configure authentication, and run the first list, export, upload, and delete workflow.

## Prerequisites

- Go toolchain installed locally.
- An authenticated Feishu/Lark account that can access Feishu/Lark Minutes.
- A valid browser `Cookie` header for the Feishu/Lark Minutes web session.

Do not commit real cookies, HAR files, or personal config files. Treat the cookie like a session secret.

## Build From Source

From the repository root:

```sh
go build -o ./openminutes .
./openminutes --help
```

You can either run the local binary with `./openminutes` or place it on your `PATH` and run `openminutes`.

## Create Configuration

Commands that call Feishu/Lark APIs require a config file. OpenMinutes creates a template automatically the first time a config-backed command runs.

The default help text points to:

```txt
~/.config/openminutes/config.toml
```

When `XDG_CONFIG_HOME` is set, OpenMinutes uses:

```txt
$XDG_CONFIG_HOME/openminutes/config.toml
```

You can also choose an explicit path:

```sh
openminutes --config ./config.toml list
```

## Configure Authentication

Create or edit the config file:

```toml
base_url = "https://meetings.feishu.cn"
space_base_url = "https://internal-api-space.feishu.cn"
cookie = "your browser Cookie header"
```

Config fields:

| Field | Required | Description |
| --- | --- | --- |
| `base_url` | No | Feishu/Lark Minutes web base URL. Defaults to `https://meetings.feishu.cn` when empty. |
| `space_base_url` | No | Feishu/Lark Minutes space API base URL. Defaults to `https://internal-api-space.feishu.cn` when empty. |
| `cookie` | Yes | Browser `Cookie` header from an authenticated Feishu/Lark Minutes session. |

To obtain the cookie, sign in to Feishu/Lark in a browser, open the browser developer tools, inspect a request to the Minutes site, and copy the request `Cookie` header into your local config.

## Environment Overrides

Environment variables override the config file:

```sh
export OPENMINUTES_BASE_URL="https://meetings.feishu.cn"
export OPENMINUTES_SPACE_BASE_URL="https://internal-api-space.feishu.cn"
export OPENMINUTES_COOKIE="your browser Cookie header"
```

Supported variables:

| Variable | Overrides |
| --- | --- |
| `OPENMINUTES_BASE_URL` | `base_url` |
| `OPENMINUTES_SPACE_BASE_URL` | `space_base_url` |
| `OPENMINUTES_COOKIE` | `cookie` |

An empty `OPENMINUTES_COOKIE` still overrides the file value and causes validation to fail.

## First Workflow

List the first page of Minutes:

```sh
openminutes list
```

Export a transcript to stdout:

```sh
openminutes get <token>
```

Export a subtitle file without overwriting an existing file:

```sh
openminutes get <token> --file_type srt --speaker --timestamp -O meeting.srt
```

Upload a supported media file:

```sh
openminutes upload ./meeting.mp4
```

Move a Minute to trash:

```sh
openminutes delete <token> --yes
```

Permanently delete a Minute after moving it to trash:

```sh
openminutes delete <token> --yes --destroy
```

## Common Global Flags

| Flag | Description |
| --- | --- |
| `--config <path>` | Load a specific TOML config file. |
| `--verbose` | Enable debug logging on stderr. |
| `-h, --help` | Show command help. |
| `-v, --version` | Print the OpenMinutes version. |

## Safety Notes

- `delete` always requires `--yes`.
- `delete --destroy` performs permanent deletion after moving the Minute to trash.
- `get -O` creates files exclusively and never overwrites an existing output path.
- Store cookies only in local files or secret managers with appropriate permissions.
