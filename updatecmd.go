package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/krank56/jenklod-batman/internal/changelog"
	"github.com/krank56/jenklod-batman/internal/config"
	"github.com/krank56/jenklod-batman/internal/ui"
	"github.com/krank56/jenklod-batman/internal/update"
)

//go:embed CHANGELOG.md
var changelogMD string

const updateUsage = `Usage: %[1]s update [--check]

Installs the latest release the way this copy was installed: Homebrew,
go install, or the release archive (checked against checksums.txt), then
prints what changed. --check only says whether there is a newer release.`

// runUpdate is the `update` subcommand.
func runUpdate(current string, fromBuildInfo bool, args []string) error {
	check := false
	for _, a := range args {
		switch a {
		case "--check", "-check":
			check = true
		case "-h", "--help":
			fmt.Println(usage(updateUsage))
			return nil
		default:
			return fmt.Errorf("unknown argument %s\n\n%s", a, usage(updateUsage))
		}
	}
	cur, ok := update.Parse(current)
	if !ok {
		return fmt.Errorf("%s is a development build: update it from its source, or install a release", current)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	src := update.NewSource()
	latest, err := src.Latest(ctx)
	if err != nil {
		return fmt.Errorf("asking GitHub for the latest release: %w", err)
	}
	st := loadUpdateState()
	if st != nil {
		st.Record(latest, time.Now())
		st.Save() // best effort: it only spares the TUI a check
	}
	to, ok := update.Parse(latest.Tag)
	if !ok {
		return fmt.Errorf("the latest release has an unexpected tag: %q", latest.Tag)
	}
	if !cur.Less(to) {
		fmt.Printf("jenklod-batman %s is the latest release.\n", cur)
		return nil
	}
	if check {
		fmt.Printf("jenklod-batman %s is out (released %s; you have %s). Run %s update.\n",
			to, latest.Published.Format("2006-01-02"), cur, cmd)
		return nil
	}

	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return fmt.Errorf("finding this binary: %w", err)
	}
	how := update.Detect(exe, fromBuildInfo)
	fmt.Printf("Updating jenklod-batman %s → %s with %s…\n", cur, to, how)
	in := update.NewInstaller(exe)
	in.Source = src
	if err := in.Install(ctx, how, to); err != nil {
		return err
	}
	if st != nil {
		st.Seen = to.String() // the notes are printed here: no what's new in the TUI
		st.Save()
	}
	fmt.Printf("\nUpdated to %s.\n", to)
	if rels, err := src.Releases(ctx); err == nil {
		for _, r := range update.Between(rels, cur, to) {
			fmt.Printf("\n## %s\n\n%s\n", r.Tag, strings.TrimSpace(r.Notes))
		}
	}
	return nil
}

const changelogUsage = `Usage: %[1]s changelog [VERSION]

Prints the changelog, or the section of one version (e.g. v0.4.0, or
Unreleased). Exits non-zero if that version has no section.`

// runChangelog is the `changelog` subcommand; the release workflow takes
// a tag's section from it as the release notes.
func runChangelog(args []string) error {
	switch {
	case len(args) == 0:
		fmt.Print(changelogMD)
		return nil
	case len(args) == 1 && (args[0] == "-h" || args[0] == "--help"):
		fmt.Println(usage(changelogUsage))
		return nil
	case len(args) == 1:
		s, ok := changelog.Section(changelogMD, args[0])
		if !ok {
			return fmt.Errorf("CHANGELOG.md has no section for %s", args[0])
		}
		fmt.Println(s)
		return nil
	}
	return errors.New(usage(changelogUsage))
}

func loadUpdateState() *update.State {
	path, err := update.StatePath()
	if err != nil {
		return nil
	}
	st, _ := update.LoadState(path)
	return st
}

// startupUpdates gives the TUI its update check and, after an upgrade,
// what's new. Development builds get neither.
func startupUpdates(cfg *config.Config, current string) (*update.Checker, ui.WhatsNew) {
	v, ok := update.Parse(current)
	st := loadUpdateState()
	if !ok || st == nil {
		return nil, ui.WhatsNew{}
	}
	// Upgraded: from an older version that ran here, or from one too old
	// to have said (a config already set up, but nothing seen yet).
	seen, seenOK := update.Parse(st.Seen)
	var news ui.WhatsNew
	if seenOK && seen.Less(v) || st.Seen == "" && cfg.Complete() {
		if notes, ok := changelog.Section(changelogMD, v.String()); ok {
			news = ui.WhatsNew{Version: v.String(), Notes: notes}
		}
	}
	if st.Seen != v.String() {
		st.Seen = v.String()
		st.Save()
	}
	if !cfg.UpdatesOn() {
		return nil, news
	}
	return &update.Checker{Source: update.NewSource(), State: st, Current: v, Now: time.Now}, news
}
