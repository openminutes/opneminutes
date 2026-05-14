# `openminutes upload`

## Purpose

Upload one local audio or video file to Feishu/Lark Minutes for transcription.

OpenMinutes validates the file path, extension, size, and duration when duration probing is available. A successful upload prints the created Minute token and URL.

## Usage

```sh
openminutes upload FILE [flags]
```

## Arguments

| Argument | Description |
| --- | --- |
| `FILE` | Required path to one local audio or video file. Exactly one file is accepted. |

## Flags

`upload` has no command-specific flags.

Global flags such as `--config` and `--verbose` are available.

## Supported Files

Supported extensions:

```txt
.wav, .mp3, .m4a, .aac, .ogg, .wma, .amr, .avi, .wmv, .mov, .mp4, .m4v, .mpeg, .flv
```

Limits:

| Limit | Value |
| --- | --- |
| Maximum file size | 6 GiB |
| Maximum duration | 6 hours when duration probing is available |

Duration probing is implemented for `.mp4`, `.m4v`, `.m4a`, `.mov`, `.wav`, `.mp3`, and `.ogg`. For other supported extensions, duration validation is skipped when the duration is unknown.

## Examples

```sh
openminutes upload ./meeting.mp4
openminutes --verbose upload ./interview.mp3
```

## Output

Successful uploads print:

```txt
Uploaded m_abc123 https://meetings.feishu.cn/minutes/m_abc123
```

The URL is built from `base_url` and the returned object token.

## Errors And Notes

- Requires a valid config and cookie.
- `FILE` is required and must not be blank.
- Only one file path is accepted.
- The file must exist and must not be a directory.
- Unsupported file extensions fail before upload.
- Files larger than 6 GiB fail before upload.
- Files longer than 6 hours fail when duration probing is available and succeeds.
