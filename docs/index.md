---
layout: home

hero:
  name: "OpenMinutes"
  text: "A Go CLI for Feishu/Lark Minutes"
  tagline: List, export, upload, and delete Minutes from a terminal workflow.
  actions:
    - theme: brand
      text: Quick Start
      link: /quick-start
    - theme: alt
      text: CLI Guides
      link: /cli/openminutes

features:
  - title: List Minutes
    details: Browse Minutes in the authenticated Feishu/Lark account, page through results, or fetch every available item.
  - title: Export Text
    details: Export a Minute as txt or srt, optionally including speakers and timestamps, with stdout, file, and JSON output modes.
  - title: Upload Media
    details: Upload supported audio and video files for transcription and receive the created Minute token and URL.
  - title: Delete Safely
    details: Move one or more Minutes to trash, or permanently delete them only when explicitly confirmed.
  - title: Structured Output
    details: Use JSON output for automation around list and get workflows.
  - title: Configurable Runtime
    details: Configure Feishu/Lark endpoints and authentication with a TOML config file or environment overrides.
---

## What Is OpenMinutes?

OpenMinutes is a command-line tool for working with Feishu/Lark Minutes. It is built in Go and focuses on repeatable terminal workflows: listing Minutes, exporting transcript text, uploading local media for transcription, and deleting Minutes from the authenticated account.

Start with the [Quick Start](/quick-start) to build the binary, configure authentication, and run the first workflow. For command-specific behavior, see the [CLI guides](/cli/openminutes).
