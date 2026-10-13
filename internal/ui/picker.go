package ui

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/krank56/jenklod-batman/internal/github"
	"github.com/krank56/jenklod-batman/internal/jenkins"
)

// pickerRows is how many branches the dropdown shows at once.
const pickerRows = 8

// textFocused reports whether the focused field takes free text, and so
// can take a branch.
func (p paramsState) textFocused() bool {
	if len(p.fields) == 0 {
		return false
	}
	switch p.fields[p.focus].def.Type {
	case jenkins.ParamString, jenkins.ParamText:
		return true
	}
	return false
}

// openPicker opens the branch dropdown on the focused field. The job's
// repository comes from the config, else from what its last build checked
// out.
func (m Model) openPicker() (tea.Model, tea.Cmd) {
	p := &m.params
	p.pick = &branchPicker{field: p.focus}
	job := p.job.FullName()
	if s := m.cfg.Repo(job); s != "" {
		if r, ok := github.ParseRepo(s); ok {
			return m, m.pickRepo(r)
		}
		m.setStatus(fmt.Sprintf("The repo set for %s in the config is not owner/name: %q", job, s), true)
	}
	if r, ok := m.jobRepos[job]; ok {
		return m, m.pickRepo(r)
	}
	return m, m.loadRemotes(p.job)
}

// pickRepo shows the branches of r: the last list at once, if any, while
// a fresh one is fetched.
func (m *Model) pickRepo(r github.Repo) tea.Cmd {
	pk := m.params.pick
	pk.repo, pk.choices, pk.ask, pk.askErr = r, nil, nil, ""
	pk.branches, pk.loading, pk.cursor = m.branches[r], true, 0
	m.params.focusField(m.params.focus) // back from typing the repo
	return m.loadBranches(r)
}

// chooseRepo is the user's answer to which repository the job builds; it
// is saved so the picker never asks again.
func (m Model) chooseRepo(r github.Repo) (tea.Model, tea.Cmd) {
	job := m.params.job.FullName()
	m.cfg.SetRepo(job, r.String())
	if err := m.cfg.Save(); err != nil {
		m.setErr(fmt.Errorf("saving the repo of %s: %w", job, err))
	}
	m.jobRepos[job] = r
	return m, m.pickRepo(r)
}

func (m *Model) closePicker() {
	m.params.pick = nil
	m.params.focusField(m.params.focus)
}

// pickerFor is the open picker for job, or nil if it was closed or the
// form has changed since.
func (m Model) pickerFor(job string) *branchPicker {
	if m.scr != scrParams || m.params.job.FullName() != job {
		return nil
	}
	return m.params.pick
}

func (m Model) handleRepos(msg reposMsg) (tea.Model, tea.Cmd) {
	pk := m.pickerFor(msg.job)
	if msg.err != nil {
		if pk != nil {
			m.closePicker()
		}
		m.setErr(fmt.Errorf("finding the repo of %s: %w", msg.job, msg.err))
		return m, nil
	}
	var repos []github.Repo
	for _, u := range msg.urls {
		if r, ok := github.ParseRepo(u); ok && !slices.Contains(repos, r) {
			repos = append(repos, r)
		}
	}
	if len(repos) == 1 {
		m.jobRepos[msg.job] = repos[0]
	}
	if pk == nil || pk.repo != (github.Repo{}) {
		return m, nil
	}
	switch len(repos) {
	case 1:
		return m, m.pickRepo(repos[0])
	case 0:
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = "owner/name"
		ti.Width = 40
		ti.Focus()
		pk.ask = &ti
		m.params.fields[m.params.focus].input.Blur()
		return m, textinput.Blink
	}
	pk.choices = repos
	return m, nil
}

func (m Model) handleBranches(msg branchesMsg) (tea.Model, tea.Cmd) {
	pk := m.params.pick
	open := m.scr == scrParams && pk != nil && pk.repo == msg.repo
	if msg.err != nil {
		if open {
			m.closePicker()
		}
		m.setErr(fmt.Errorf("branches of %s: %w", msg.repo, msg.err))
		return m, nil
	}
	m.branches[msg.repo] = msg.branches
	if open {
		pk.branches, pk.loading = msg.branches, false
		pk.cursor = clamp(pk.cursor, 0, max(0, len(m.pickerHits())-1))
	}
	return m, nil
}

func (m Model) pickerKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.params
	pk := p.pick
	switch k.String() {
	case "esc", "ctrl+g":
		m.closePicker()
		return m, nil
	case "tab", "shift+tab":
		m.closePicker()
		return m.paramsKey(k)
	}
	switch {
	case pk.ask != nil:
		if k.String() == "enter" {
			r, ok := github.ParseRepo(pk.ask.Value())
			if !ok {
				pk.askErr = "Type owner/name, or the repository's URL"
				return m, nil
			}
			return m.chooseRepo(r)
		}
		var cmd tea.Cmd
		*pk.ask, cmd = pk.ask.Update(k)
		pk.askErr = ""
		return m, cmd

	case len(pk.choices) > 0:
		switch k.String() {
		case "down", "j", "ctrl+n":
			pk.cursor = min(pk.cursor+1, len(pk.choices)-1)
		case "up", "k", "ctrl+p":
			pk.cursor = max(pk.cursor-1, 0)
		case "enter":
			return m.chooseRepo(pk.choices[pk.cursor])
		}
		return m, nil
	}

	f := &p.fields[pk.field]
	switch k.String() {
	case "down", "ctrl+n":
		pk.cursor = min(pk.cursor+1, max(0, len(m.pickerHits())-1))
		return m, nil
	case "up", "ctrl+p":
		pk.cursor = max(pk.cursor-1, 0)
		return m, nil
	case "enter":
		// Nothing matches: keep what was typed (a SHA, a branch just pushed).
		if hits := m.pickerHits(); len(hits) > 0 {
			f.input.SetValue(hits[pk.cursor].Name)
			f.input.CursorEnd()
		}
		m.closePicker()
		return m, nil
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(k)
	pk.cursor = 0
	return m, cmd
}

// pickerHits are the branches matching what the field holds, best first.
func (m Model) pickerHits() []github.Branch {
	pk := m.params.pick
	return rankBranches(m.params.fields[pk.field].input.Value(), pk.branches)
}

// rankBranches keeps the branches whose name matches pat, best first, in
// their given order (default, then newest) on a tie.
func rankBranches(pat string, bs []github.Branch) []github.Branch {
	if strings.TrimSpace(pat) == "" {
		return bs
	}
	type hit struct {
		b     github.Branch
		score int
	}
	var hits []hit
	for _, b := range bs {
		if s, ok := fuzzyScore(pat, b.Name); ok {
			hits = append(hits, hit{b, s})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	out := make([]github.Branch, len(hits))
	for i, h := range hits {
		out[i] = h.b
	}
	return out
}

// pickerView is the dropdown, drawn under the field it fills.
func (m Model) pickerView() string {
	pk := m.params.pick
	job := m.params.job.FullName()
	var b strings.Builder
	spin := sKey.Render(spinner[m.frame%len(spinner)])
	foot := "enter pick · esc keep typed text"
	nameW := min(48, max(10, m.w-40))
	width := 0 // as wide as the text, except the branch list: steady while filtering
	switch {
	case pk.ask != nil:
		b.WriteString(sLabel.Render("Which GitHub repository does "+job+" build?") + "\n")
		b.WriteString(sDim.Render("Its last build checked out none. Saved in the config.") + "\n\n")
		b.WriteString(pk.ask.View() + "\n")
		if pk.askErr != "" {
			b.WriteString(sErr.Render(pk.askErr) + "\n")
		}
		foot = "enter save · esc cancel"
	case len(pk.choices) > 0:
		b.WriteString(sLabel.Render("Which repository does "+job+" build?") + "\n")
		b.WriteString(sDim.Render("Its last build checked out several. Saved in the config.") + "\n\n")
		for i, r := range pk.choices {
			if i == pk.cursor {
				b.WriteString(sCursor.Render("▸ ") + sSelected.Render(r.String()) + "\n")
			} else {
				b.WriteString("  " + r.String() + "\n")
			}
		}
		foot = "enter choose · esc cancel"
	case pk.repo == (github.Repo{}):
		b.WriteString(spin + sDim.Render(" finding the repository "+job+" builds…") + "\n")
	default:
		title := sDim.Render("branches · ") + sLabel.Render(pk.repo.String())
		if pk.loading {
			title += " " + spin
		}
		b.WriteString(title + "\n")
		hits := m.pickerHits()
		switch {
		case len(pk.branches) == 0 && pk.loading:
			b.WriteString(sDim.Render("fetching…") + "\n")
		case len(hits) == 0:
			b.WriteString(sDim.Render("no match: enter keeps what you typed") + "\n")
		}
		width = nameW + 24 // cursor, name, date, tag, padding
		start, end := window(pk.cursor, len(hits), pickerRows)
		for i := start; i < end; i++ {
			br := hits[i]
			name := pad(truncate(br.Name, nameW), nameW)
			when := ""
			if !br.Committed.IsZero() {
				when = fmtAgo(br.Committed)
			}
			tag := ""
			if br.Default {
				tag = sPin.Render(" default")
			}
			if i == pk.cursor {
				b.WriteString(sCursor.Render("▸ ") + sSelected.Render(name) + " " + sDim.Render(pad(when, 9)) + tag + "\n")
			} else {
				b.WriteString("  " + name + " " + sDim.Render(pad(when, 9)) + tag + "\n")
			}
		}
		if len(hits) > pickerRows {
			b.WriteString(sDim.Render(fmt.Sprintf("  %d/%d", pk.cursor+1, len(hits))) + "\n")
		}
	}
	b.WriteString(sDim.Render(foot))
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(dim).Padding(0, 1)
	if width > 0 {
		style = style.Width(width)
	}
	box := style.Render(b.String())
	return lipgloss.NewStyle().MarginLeft(4).Render(box)
}
