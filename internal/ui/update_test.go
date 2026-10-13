package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/krank56/jenklod-batman/internal/jenkins"
	"github.com/krank56/jenklod-batman/internal/update"
)

// noJenkins is a client for tests that never reach Jenkins: past the
// setup form, without serving anything.
func noJenkins() *jenkins.Client {
	c, _ := jenkins.New("http://127.0.0.1:1", "bruce", "fake-test-value")
	return c
}

func TestUpdateCheckAtStart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v0.4.0","published_at":%q}`, time.Now().Add(-2*time.Hour).Format(time.RFC3339))
	}))
	defer srv.Close()
	st, _ := update.LoadState(filepath.Join(t.TempDir(), "update.json"))
	cur, _ := update.Parse("v0.3.0")
	js := httptest.NewServer(&fakeJenkins{})
	defer js.Close()
	client, _ := jenkins.New(js.URL, "bruce", "fake-test-value")
	m := New(Options{Config: newCfg(t), Client: client, NoAnim: true, Command: "jk", Updates: &update.Checker{
		Source: &update.Source{API: srv.URL, HTTP: srv.Client()}, State: st, Current: cur, Now: time.Now,
	}})
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = drive(t, m, cmdMsg{m.Init()})
	v := snapshot(t, "50-update-badge", m)
	if !strings.Contains(v, "↑ v0.4.0") || !strings.Contains(m.status, "v0.4.0 is out") || !strings.Contains(m.status, "run jk update") {
		t.Errorf("status=%q\n%s", m.status, v)
	}
}

func TestUpdateSaidOnceAndAfterTheSplash(t *testing.T) {
	v4, _ := update.Parse("v0.4.0")
	m := New(Options{Config: newCfg(t), Client: noJenkins()}) // with the splash, then the jobs
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = drive(t, m, updateMsg{v: v4, news: true})
	if m.status != "" {
		t.Fatalf("said under the splash, where nobody sees it: %q", m.status)
	}
	m = drive(t, m, key("x")) // skip the splash
	if !strings.Contains(m.status, "v0.4.0 is out") {
		t.Errorf("status = %q", m.status)
	}

	m.setStatus("something else", false)
	m = drive(t, m, updateMsg{v: v4})
	if m.status != "something else" || !strings.Contains(m.View(), "↑ v0.4.0") {
		t.Errorf("not news: the badge only, status = %q", m.status)
	}
}

func TestUpdateCheckErrorsAreQuiet(t *testing.T) {
	m := New(Options{Config: newCfg(t), NoAnim: true})
	m = drive(t, m, updateMsg{err: fmt.Errorf("offline")})
	if m.status != "" || m.newVersion != "" {
		t.Errorf("status=%q badge=%q", m.status, m.newVersion)
	}
}

func TestWhatsNewOnce(t *testing.T) {
	m := New(Options{Config: newCfg(t), Client: noJenkins(), NoAnim: true, WhatsNew: WhatsNew{
		Version: "v0.4.0",
		Notes:   "### Builds\n- A build you start shows up in the list on its own.\n- `ctrl+g` picks a **branch**.",
	}})
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	v := snapshot(t, "51-whats-new", m)
	for _, want := range []string{"What's new in v0.4.0", "Builds", "• A build you start shows up", "ctrl+g picks a branch"} {
		if !strings.Contains(v, want) {
			t.Errorf("lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "###") || strings.Contains(v, "`") || strings.Contains(v, "**") {
		t.Errorf("markdown left raw:\n%s", v)
	}
	next, cmd := m.Update(key("q"))
	m = next.(Model)
	if strings.Contains(m.View(), "What's new") || cmd != nil {
		t.Error("any key closes it, and does nothing else")
	}
}

func TestLongStatusTakesTheFooter(t *testing.T) {
	m := New(Options{Config: newCfg(t), Client: noJenkins(), NoAnim: true})
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	note := "🦇 v0.4.0 is out — run jenklod-batman update"
	m.setStatus(note, false)
	foot := m.footerView()
	if !strings.Contains(foot, note) || strings.Contains(foot, "rebuild") {
		t.Errorf("an info status that doesn't fit gets the line alone:\n%s", foot)
	}
	m.setStatus("Jenkins said no: "+strings.Repeat("x", 80), true)
	if foot := m.footerView(); !strings.Contains(foot, "rebuild") {
		t.Errorf("an error keeps the hints, since it stays:\n%s", foot)
	}
	m.setStatus("short", false)
	if foot := m.footerView(); !strings.Contains(foot, "rebuild") || !strings.Contains(foot, "short") {
		t.Errorf("a status that fits sits beside the hints:\n%s", foot)
	}
}
