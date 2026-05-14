# OpenMinutes

OpenMinutes is a Go CLI for Feishu/Lark Minutes. It supports terminal workflows for listing Minutes, exporting transcript text, uploading media for transcription, and deleting Minutes from the authenticated account.

## Installation

Install from npm:

```sh
npm install -g @openminutes/cli
```

Or build from source:

```sh
go build -o ./openminutes .
./openminutes --help
```

Release binaries are available from the GitHub releases page.

## Quick Start

Create or edit the config file:

```toml
base_url = "https://meetings.feishu.cn"
space_base_url = "https://internal-api-space.feishu.cn"
cookie = "your browser Cookie header"
```

By default, OpenMinutes reads:

```txt
~/.config/openminutes/config.toml
```

You can also pass a config file explicitly:

```sh
openminutes --config ./config.toml list
```

## Examples

List Minutes:

```sh
openminutes list
```

Export transcript text:

```sh
openminutes get <token>
```

Export an SRT file:

```sh
openminutes get <token> --file_type srt --speaker --timestamp -O meeting.srt
```

Upload media for transcription:

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

## Security

The `cookie` value is a session secret copied from an authenticated browser request. Do not commit real cookies, HAR files, tokens, or personal config files. Store local config files with restricted permissions and rotate the browser session if the cookie is exposed.

## Documentation

See the full docs at <https://openminutes.duckduckapp.com/> and the CLI reference at <https://openminutes.duckduckapp.com/cli/openminutes.html>.
