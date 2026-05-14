# `openminutes list`

## Purpose

List Minutes from the authenticated Feishu/Lark account.

By default, `list` requests one page and prints a follow-up command when another page is available. Use `--timestamp` to continue from that page, or `--all` to follow pagination automatically.

## Usage

```sh
openminutes list [flags]
```

## Arguments

`list` does not accept positional arguments.

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--size int` | `20` | Number of Minutes to request per page. Must be greater than `0`. |
| `--timestamp int` | `0` | Pagination timestamp to start from. Must be greater than or equal to `0`. |
| `--json` | `false` | Print structured JSON instead of plain rows. |
| `--all` | `false` | Follow pagination and list all Minutes, starting from `--timestamp` when provided. |

Global flags such as `--config` and `--verbose` are also available.

## Examples

```sh
openminutes list
openminutes list --size 50 --timestamp 1710000000
openminutes list --all --json
openminutes --config ./config.toml list --json
```

## Output

Plain output starts with the row format, then prints one row per Minute:

```txt
Columns: token name URL
m_abc123 Weekly sync https://meetings.feishu.cn/minutes/m_abc123

Next page: openminutes list --size 20 --timestamp 1710000000
Get content: openminutes get <token>
```

If there are no results, the command prints:

```txt
No minutes found.
```

JSON output has this shape:

```json
{
  "items": [
    {
      "object_token": "m_abc123",
      "object_type": 1,
      "topic": "Weekly sync",
      "url": "https://meetings.feishu.cn/minutes/m_abc123",
      "media_type": "video",
      "owner_name": "Ada",
      "duration": 1800,
      "share_time": 1710000000,
      "start_time": 1710000000,
      "stop_time": 1710001800,
      "create_time": 1710000000,
      "status": 0
    }
  ],
  "has_more": true,
  "next_timestamp": 1710000000
}
```

When `--all` is used, OpenMinutes returns all collected items and does not print a next-page command in plain output.

## Errors And Notes

- Requires a valid config and cookie.
- `--size 0` or a negative size fails before the API call.
- A negative `--timestamp` fails before the API call.
- If a list item has no topic, plain output prints `(untitled)`.
- If a list item has no URL, OpenMinutes builds one from `base_url` and the object token.
