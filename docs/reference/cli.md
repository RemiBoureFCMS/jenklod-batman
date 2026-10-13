# Command line

## `jenklod-batman`

Starts the TUI.

| Flag | Default | Description |
|---|---|---|
| `--config FILE` | `~/.config/jenklod-batman/config.toml` | Config file. `$XDG_CONFIG_HOME` replaces `~/.config`. |
| `--setup` | | Open the setup form again (URL, user, token). |
| `--forget-token` | | Remove the stored token from the keyring and exit. |
| `--no-anim` | | Skip the splash and keep the cat still. |
| `--version` | | Print the version and exit. |

## `jenklod-batman <macro>`, `jenklod-batman run`

Runs a [macro](/guide/macros) without the TUI. `run` is optional: `jenklod-batman ship` and `jenklod-batman run ship` are the same.

```text
jenklod-batman <macro> [VALUE] [KEY=VALUE…] [-y] [--config FILE]
jenklod-batman run <macro> [VALUE] [KEY=VALUE…] [-y] [--config FILE]
jenklod-batman run --list
```

| Argument | Description |
|---|---|
| `<macro>` | The macro's name, as listed by `--list`. |
| `VALUE` | Fills the macro's [argument](/guide/macros#an-argument) parameter. An error if the macro has none, or if `KEY=VALUE` sets the same parameter. |
| `KEY=VALUE` | Override that parameter in every build step that has it. Repeat for several. A key that no build step uses is an error. |
| `-y`, `--yes` | Don't ask for confirmation. |
| `-l`, `--list` | List the macros of the configured Jenkins, with their steps. |
| `--config FILE` | Config file, as for the TUI. |
| `-h`, `--help` | Show the usage. |

**Behaviour:**

- **Before running,** it prints the steps and any overrides. On a terminal it asks `Run it? [y/N]`. With `-y`, or without a terminal, it doesn't ask.
- **Progress** goes to standard output as `[step/total] message`.
- **Exit codes:** `0` when every step succeeded; `1` on any error, including a failed step, an unknown macro or an unused override.
- **Without `run`,** the first word must not start with `-`: anything else starts the TUI with its flags. A macro named `run`, `update` or `changelog` needs `run` in front, since those are commands.
- **Interrupting:** <kbd>ctrl+c</kbd> or `SIGTERM` stops the run. Builds already started keep running.

## `jenklod-batman update`

Installs the latest release.

```text
jenklod-batman update [--check]
```

| Argument | Description |
|---|---|
| `--check` | Only say whether there's a newer release. |
| `-h`, `--help` | Show the usage. |

It updates the way this copy was installed:

| Installed with | Update |
|---|---|
| Homebrew | `brew upgrade krank56/tap/jenklod-batman`. If the tap doesn't have the release yet, it says so: it catches up within the hour. |
| `go install` | `go install github.com/krank56/jenklod-batman@<version>`. |
| A release archive | Downloads the archive for this OS and architecture, checks it against the release's `checksums.txt`, and replaces the binary. It needs write access to the binary's directory. |

Then it prints the release notes of every version since yours. A development build (`dev`, or a version like `v0.3.0-12-gabc1234` from `make build`) refuses to update.

## `jenklod-batman changelog`

```text
jenklod-batman changelog [VERSION]
```

Prints the [changelog](/changelog), which is built into the binary, or only the section of `VERSION` (`v0.3.0`, or `Unreleased`). Exits `1` if there's no such section.

## Environment

| Variable | Description |
|---|---|
| `JENKINS_TOKEN` | API token to use instead of the keyring's. |
| `XDG_CONFIG_HOME` | Base directory for the config (default `~/.config`). |
| `JB_GITHUB_API` | Send the branch picker's queries to this GraphQL endpoint instead of GitHub's, with no token. For the [demo Jenkins](/contributing/development#the-demo-jenkins). |
| `XDG_CACHE_HOME` | Base directory for the update check's state, `jenklod-batman/update.json` (default `~/.cache`). |
