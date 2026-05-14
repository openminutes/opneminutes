# `openminutes get`

## Purpose

Export one Minute as transcript text or subtitles.

The exported content is printed to stdout by default. Use `-O, --output` to write it to a file. Output files are created exclusively and existing files are never overwritten.

## Usage

```sh
openminutes get TOKEN [flags]
```

## Arguments

| Argument | Description |
| --- | --- |
| `TOKEN` | Required object token for the Minute to export. Exactly one token is accepted. |

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--file_type string` | `txt` | Export format. Supported values are `txt` and `srt`. |
| `--speaker` | `false` | Include speaker names in the exported text. |
| `--timestamp` | `false` | Include timestamps in the exported text. |
| `-O, --output string` | empty | Write exported content to this file path. |
| `--json` | `false` | Print structured JSON instead of plain text or the default saved-file message. |

Global flags such as `--config` and `--verbose` are also available.

## Examples

```sh
openminutes get m_abc123
openminutes get m_abc123 --json
openminutes get m_abc123 --file_type srt --speaker --timestamp -O meeting.srt
openminutes get m_abc123 -O transcript.txt --json
```

## Output

Without `--output`, plain output is the exported content. OpenMinutes adds a trailing newline when the exported content does not already end with one.

With `--output`, plain output confirms the saved file:

```txt
Saved m_abc123 to meeting.srt
```

With `--json` and no output file, the exported content is included inline:

```json
{
  "object_token": "m_abc123",
  "file_type": "txt",
  "speaker": false,
  "timestamp": false,
  "bytes": 128,
  "content": "Transcript content"
}
```

With `--json` and `--output`, the content is written to disk and the JSON contains metadata:

```json
{
  "object_token": "m_abc123",
  "file_type": "srt",
  "speaker": true,
  "timestamp": true,
  "bytes": 256,
  "output_path": "meeting.srt"
}
```

## Errors And Notes

- Requires a valid config and cookie.
- `TOKEN` is required and must not be blank.
- Only one token is accepted.
- `--file_type` must be `txt` or `srt`.
- If `--output` is passed with an empty value, the command fails before the API call.
- If the output path already exists, the command fails before the API call.
- If writing the output file fails, OpenMinutes removes the partial file.
