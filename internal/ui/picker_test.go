package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/krank56/jenklod-batman/internal/github"
	"github.com/krank56/jenklod-batman/internal/jenkins"
)

// pickerFakes are a Jenkins whose last builds checked out remotes, and a
// GitHub serving branches per repo, newest first.
type pickerFakes struct {
	mu       sync.Mutex
	remotes  map[string][]string // job name -> remote URLs
	branches map[string][]string // owner/name -> branches, newest first
	detects  int
	fetched  []string // repos asked of GitHub
}

func (f *pickerFakes) jenkins(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/job/"), "/api/json")
	f.detects++
	urls, _ := json.Marshal(f.remotes[name])
	fmt.Fprintf(w, `{"lastBuild":{"actions":[{"remoteUrls":%s}]}}`, urls)
}

func (f *pickerFakes) github(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var req struct {
		Variables map[string]any `json:"variables"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	repo := fmt.Sprintf("%s/%s", req.Variables["owner"], req.Variables["name"])
	f.fetched = append(f.fetched, repo)
	var nodes []string
	for i, b := range f.branches[repo] {
		d := time.Now().Add(-time.Duration(i+1) * time.Hour)
		nodes = append(nodes, fmt.Sprintf(`{"name":%q,"target":{"committedDate":%q}}`, b, d.Format(time.RFC3339)))
	}
	fmt.Fprintf(w, `{"data":{"repository":{"defaultBranchRef":{"name":"main"},"refs":{"pageInfo":{"hasNextPage":false},"nodes":[%s]}}}}`,
		strings.Join(nodes, ","))
}

// pickerModel opens the build form of "deploy": a choice, a text field
// (Version) and a password.
func pickerModel(t *testing.T, f *pickerFakes) Model {
	t.Helper()
	js := httptest.NewServer(http.HandlerFunc(f.jenkins))
	gs := httptest.NewServer(http.HandlerFunc(f.github))
	t.Cleanup(js.Close)
	t.Cleanup(gs.Close)
	client, _ := jenkins.New(js.URL, "bruce", "fake-test-value")
	cfg := newCfg(t)
	cfg.URL = js.URL
	m := New(Options{Config: cfg, Client: client, NoAnim: true})
	m.gh = &github.Client{
		API:   func(string) string { return gs.URL },
		Token: func(string) (string, error) { return "gh-test-value", nil },
		HTTP:  gs.Client(),
	}
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	job := jenkins.Job{Name: "deploy", Path: []string{"deploy"}, Class: "org.jenkinsci.plugins.workflow.job.WorkflowJob"}
	m.builds.job = job
	m.params = newParams(job, []jenkins.Param{
		{Name: "ENV", Type: jenkins.ParamChoice, Choices: []string{"staging", "prod"}},
		{Name: "Version", Description: "Version/branche à préciser"},
		{Name: "PW", Type: jenkins.ParamPassword},
	})
	m.params.returnTo = scrBuilds
	m.scr = scrParams
	return drive(t, m, key("tab")) // Version
}

func ctrlG() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlG} }

func gothamRepo() *pickerFakes {
	return &pickerFakes{
		remotes:  map[string][]string{"deploy": {"git@github.com:wayne/gotham.git"}},
		branches: map[string][]string{"wayne/gotham": {"feat/checkout-v2", "main", "feat/cheap-shipping", "fix/check-tax"}},
	}
}

func TestPickerFillsTheField(t *testing.T) {
	m := pickerModel(t, gothamRepo())
	if v := m.View(); !strings.Contains(v, "ctrl+g") {
		t.Errorf("a text field should hint ctrl+g:\n%s", v)
	}
	m = drive(t, m, ctrlG())
	v := snapshot(t, "40-branch-picker", m)
	for _, want := range []string{"wayne/gotham", "main", "default", "feat/checkout-v2", "fix/check-tax"} {
		if !strings.Contains(v, want) {
			t.Errorf("picker lacks %q:\n%s", want, v)
		}
	}
	if strings.Index(v, "main") > strings.Index(v, "feat/checkout-v2") {
		t.Errorf("the default branch comes first:\n%s", v)
	}

	m = typeText(t, m, "chko")
	v = snapshot(t, "41-branch-picker-filtered", m)
	if !strings.Contains(v, "feat/checkout-v2") || strings.Contains(v, "fix/check-tax") {
		t.Errorf("typing filters:\n%s", v)
	}
	m = drive(t, m, key("enter"))
	if m.params.pick != nil || m.scr != scrParams {
		t.Fatalf("enter picks and closes, screen=%d", m.scr)
	}
	if got := m.params.values()["Version"]; got != "feat/checkout-v2" {
		t.Errorf("Version = %q", got)
	}
}

func TestPickerKeepsTypedText(t *testing.T) {
	m := pickerModel(t, gothamRepo())
	m = drive(t, m, ctrlG())
	m = typeText(t, m, "deadbeef")
	if v := m.View(); !strings.Contains(v, "no match") {
		t.Errorf("should say nothing matches:\n%s", v)
	}
	m = drive(t, m, key("enter"))
	if got := m.params.values()["Version"]; got != "deadbeef" || m.params.pick != nil {
		t.Errorf("Version = %q, open = %v", got, m.params.pick != nil)
	}

	m = drive(t, m, ctrlG())
	m = drive(t, m, key("esc"))
	if m.scr != scrParams || m.params.pick != nil || m.params.values()["Version"] != "deadbeef" {
		t.Errorf("esc closes the picker only: screen=%d value=%q", m.scr, m.params.values()["Version"])
	}
}

func TestPickerTabMovesOn(t *testing.T) {
	m := pickerModel(t, gothamRepo())
	m = drive(t, m, ctrlG())
	m = drive(t, m, key("tab"))
	if m.params.pick != nil || m.params.focus != 2 {
		t.Errorf("tab closes and moves on: open=%v focus=%d", m.params.pick != nil, m.params.focus)
	}
}

func TestPickerOnlyOnTextFields(t *testing.T) {
	m := pickerModel(t, gothamRepo())
	m = drive(t, m, key("tab")) // PW
	if m = drive(t, m, ctrlG()); m.params.pick != nil {
		t.Error("no picker on a password")
	}
	m = drive(t, m, key("tab")) // ENV
	if m = drive(t, m, ctrlG()); m.params.pick != nil {
		t.Error("no picker on a choice")
	}
}

func TestPickerUsesTheConfiguredRepo(t *testing.T) {
	f := gothamRepo()
	f.branches["wayne/other"] = []string{"main", "release/1"}
	m := pickerModel(t, f)
	m.cfg.SetRepo("deploy", "wayne/other")
	m = drive(t, m, ctrlG())
	if f.detects != 0 || strings.Join(f.fetched, ",") != "wayne/other" {
		t.Errorf("detects=%d fetched=%v", f.detects, f.fetched)
	}
	if v := m.View(); !strings.Contains(v, "release/1") {
		t.Errorf("picker:\n%s", v)
	}
}

func TestPickerAsksWhichRepo(t *testing.T) {
	f := gothamRepo()
	f.remotes["deploy"] = append(f.remotes["deploy"], "https://github.com/wayne/pipeline-lib.git")
	f.branches["wayne/pipeline-lib"] = []string{"main", "v2"}
	m := pickerModel(t, f)
	m = drive(t, m, ctrlG())
	v := snapshot(t, "42-branch-picker-which-repo", m)
	if !strings.Contains(v, "wayne/gotham") || !strings.Contains(v, "wayne/pipeline-lib") || len(f.fetched) != 0 {
		t.Fatalf("should ask first, fetched=%v:\n%s", f.fetched, v)
	}
	m = drive(t, m, key("down"))
	m = drive(t, m, key("enter"))
	if got := m.cfg.Repo("deploy"); got != "wayne/pipeline-lib" {
		t.Errorf("remembered %q", got)
	}
	if v := m.View(); !strings.Contains(v, "v2") {
		t.Errorf("then lists its branches:\n%s", v)
	}

	// Remembered: never asks again.
	m = drive(t, m, key("esc"))
	m = drive(t, m, ctrlG())
	if m.params.pick == nil || len(m.params.pick.choices) != 0 || f.detects != 1 {
		t.Errorf("asked again: detects=%d", f.detects)
	}
}

func TestPickerAsksForARepoWhenNoneFound(t *testing.T) {
	f := gothamRepo()
	delete(f.remotes, "deploy")
	m := pickerModel(t, f)
	m = drive(t, m, ctrlG())
	if v := m.View(); !strings.Contains(v, "owner/name") {
		t.Fatalf("should ask for the repo:\n%s", v)
	}
	m = typeText(t, m, "nope")
	m = drive(t, m, key("enter"))
	if m.params.pick == nil || m.params.pick.ask == nil || m.cfg.Repo("deploy") != "" {
		t.Fatal("an invalid repo is refused")
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m = typeText(t, m, "wayne/gotham")
	m = drive(t, m, key("enter"))
	if got := m.cfg.Repo("deploy"); got != "wayne/gotham" {
		t.Errorf("remembered %q", got)
	}
	if v := m.View(); !strings.Contains(v, "feat/checkout-v2") {
		t.Errorf("then lists its branches:\n%s", v)
	}
	if got := m.params.values()["Version"]; got != "" {
		t.Errorf("typing the repo must not touch the field: %q", got)
	}
}

func TestPickerShowsCachedBranchesThenRefreshes(t *testing.T) {
	f := gothamRepo()
	m := pickerModel(t, f)
	m = drive(t, m, ctrlG())
	m = drive(t, m, key("esc"))

	f.mu.Lock()
	f.branches["wayne/gotham"] = append([]string{"feat/brand-new"}, f.branches["wayne/gotham"]...)
	f.mu.Unlock()
	next, cmd := m.Update(ctrlG())
	mm := next.(Model)
	if cmd == nil || mm.params.pick == nil || len(mm.params.pick.branches) != 4 {
		t.Fatalf("the cached list shows at once, and a refresh starts")
	}
	mm = drive(t, mm, cmdMsg{cmd})
	if v := mm.View(); !strings.Contains(v, "feat/brand-new") {
		t.Errorf("the refresh brings the new branch:\n%s", v)
	}
}

func TestPickerWithoutGH(t *testing.T) {
	m := pickerModel(t, gothamRepo())
	m = typeText(t, m, "main")
	m.gh.Token = func(string) (string, error) { return "", github.ErrNoGH }
	m = drive(t, m, ctrlG())
	if m.params.pick != nil || !m.statusErr || !strings.Contains(m.status, "gh auth login") {
		t.Errorf("open=%v status=%q", m.params.pick != nil, m.status)
	}
	if got := m.params.values()["Version"]; got != "main" {
		t.Errorf("typed text lost: %q", got)
	}
}
