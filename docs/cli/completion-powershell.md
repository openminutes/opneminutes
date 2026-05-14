# `openminutes completion powershell`

## Purpose

Generate the PowerShell completion script for OpenMinutes.

The generated script is written to stdout. This command does not require OpenMinutes config or a Feishu/Lark cookie.

## Usage

```powershell
openminutes completion powershell [flags]
```

## Arguments

This command does not accept positional arguments.

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--no-descriptions` | `false` | Disable completion descriptions. |
| `-h, --help` | `false` | Show help for the PowerShell completion command. |

Global flags such as `--config` and `--verbose` are also available.

## Examples

Load completions in the current shell session:

```powershell
openminutes completion powershell | Out-String | Invoke-Expression
```

Add persistent completions by appending the generated script to your PowerShell profile:

```powershell
openminutes completion powershell >> $PROFILE
```

Generate without descriptions:

```powershell
openminutes completion powershell --no-descriptions
```

## Output

The command prints a PowerShell completion script to stdout.

## Errors And Notes

- Restart PowerShell after updating your profile for future sessions.
- Confirm the profile path exists before redirecting output to `$PROFILE`.
