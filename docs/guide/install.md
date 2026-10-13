# Install

jenklod-batman is a single binary for macOS and Linux, on Intel and ARM.

::: code-group

```sh [Homebrew]
brew install krank56/tap/jenklod-batman
```

```sh [Go]
go install github.com/krank56/jenklod-batman@latest
```

```sh [From a clone]
git clone https://github.com/krank56/jenklod-batman
cd jenklod-batman
make install        # into ~/go/bin (override with INSTALL_DIR=...)
```

:::

Each [release](https://github.com/krank56/jenklod-batman/releases) also has prebuilt archives (`jenklod-batman_<version>_<os>_<arch>.tar.gz`) and a `checksums.txt`.

Check that it's installed:

```sh
jenklod-batman --version
```

::: tip A different version than you expected?
A copy installed with a plain `go install` from inside a clone reports `dev` and can shadow the Homebrew one. `type -a jenklod-batman` lists every copy on your `PATH`, in the order your shell finds them.
:::

## Upgrading

```sh
jenklod-batman update
```

It installs the latest release the way this copy was installed: with `brew upgrade` for Homebrew, `go install` for Go, or by downloading the release archive and checking it against `checksums.txt`. Then it prints what changed. `jenklod-batman update --check` only says whether there's a newer release.

The TUI checks once a day and shows `↑ vX.Y.Z` in the header when there is one, an hour after it's released, which is when the Homebrew formula has caught up. `check_updates = false` in the [config](/reference/config) turns that off. The first launch after an upgrade shows what's new; `jenklod-batman changelog` prints the whole [changelog](/changelog).

Next: [First run](./first-run).
