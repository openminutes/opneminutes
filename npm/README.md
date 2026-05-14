# OpenMinutes npm packaging

This directory contains the source files for the npm distribution of the Go CLI.
The root `package.json` remains dedicated to the VitePress documentation site.

The npm release is generated into `dist/npm/` by `scripts/build-npm-packages.js`:

- `@openminutes/cli` is the main package with the `openminutes` npm bin.
- Platform packages contain one GoReleaser-built binary each and are installed as optional dependencies of the main package.
- All packages use the same version, derived from the Git tag after stripping a leading `v`.

Build and validate locally from a GoReleaser snapshot:

```sh
goreleaser release --snapshot --clean
node scripts/test-npm-packages.js --version 0.0.0-test.0
```
