package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/krank56/jenklod-batman/internal/jenkins"
)

// queueJenkins is one freestyle job whose next build waits in the queue
// (item 9) until the test starts or cancels it.
type queueJenkins struct {
	mu        sync.Mutex
	builds    []int // newest first, all finished
	queued    bool
	cancelled bool
	failQueue bool // the queue API answers 500
}

func (f *queueJenkins) start() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.builds[0] + 1
	f.builds = append([]int{n}, f.builds...)
	f.queued = false
	return n
}

func (f *queueJenkins) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/api/json":
		w.Write([]byte(`{"jobs":[{"_class":"hudson.model.FreeStyleProject","name":"lint","color":"blue"}]}`))
	case "/job/lint/api/json":
		var bs []string
		for _, n := range f.builds {
			bs = append(bs, fmt.Sprintf(`{"number":%d,"result":"SUCCESS","duration":1000,"timestamp":%d}`, n, time.Now().UnixMilli()))
		}
		q := "null"
		if f.queued {
			q = `{"id":9,"why":"In the quiet period. Expires in 4.9 sec"}`
		}
		fmt.Fprintf(w, `{"name":"lint","buildable":true,"builds":[%s],"queueItem":%s}`, strings.Join(bs, ","), q)
	case "/job/lint/build":
		f.queued, f.cancelled = true, false
		w.Header().Set("Location", "http://"+r.Host+"/queue/item/9/")
		w.WriteHeader(http.StatusCreated)
	case "/queue/item/9/api/json":
		switch {
		case f.failQueue:
			http.Error(w, "boom", http.StatusInternalServerError)
		case f.cancelled:
			w.Write([]byte(`{"cancelled":true}`))
		case f.queued:
			w.Write([]byte(`{"cancelled":false}`))
		default:
			fmt.Fprintf(w, `{"executable":{"number":%d}}`, f.builds[0])
		}
	default:
		if strings.HasSuffix(r.URL.Path, "/logText/progressiveText") {
			w.Header().Set("X-Text-Size", "9")
			w.Write([]byte("Finished\n"))
			return
		}
		http.NotFound(w, r)
	}
}

// liveModel opens lint's builds (#2, #1) on a queueJenkins.
func liveModel(t *testing.T) (Model, *queueJenkins) {
	t.Helper()
	fj := &queueJenkins{builds: []int{2, 1}}
	srv := httptest.NewServer(fj)
	t.Cleanup(srv.Close)
	client, _ := jenkins.New(srv.URL, "bruce", "fake-test-value")
	m := New(Options{Config: newCfg(t), Client: client, NoAnim: true})
	m = drive(t, m, cmdMsg{m.Init()})
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = drive(t, m, key("enter")) // lint's builds
	if m.scr != scrBuilds || m.builds.detail == nil {
		t.Fatalf("screen=%d detail=%v", m.scr, m.builds.detail)
	}
	return m, fj
}

// build presses b and confirms.
func build(t *testing.T, m Model) Model {
	t.Helper()
	m = drive(t, m, key("b"))
	return drive(t, m, key("y"))
}

// tickAt runs animation frame n without running what it starts.
func tickAt(m Model, n int) Model {
	m.frame = n - 1
	next, _ := m.Update(tickMsg{})
	return next.(Model)
}

func TestQueuedBuildShowsUpWithoutRefresh(t *testing.T) {
	m, fj := liveModel(t)
	m = build(t, m)
	v := snapshot(t, "30-queued", m)
	if !strings.Contains(v, "QUEUED") || !strings.Contains(v, "In the quiet period") {
		t.Fatalf("the queued build should have a row:\n%s", v)
	}
	if m.follows[9] == nil {
		t.Fatalf("the queue item should be followed: %v", m.follows)
	}

	n := fj.start()
	m = drive(t, m, queueTickMsg{id: 9})
	if !strings.Contains(m.status, fmt.Sprintf("lint #%d started", n)) {
		t.Errorf("status = %q", m.status)
	}
	v = snapshot(t, "31-started", m)
	if strings.Contains(v, "QUEUED") || !strings.Contains(v, fmt.Sprintf("#%d", n)) {
		t.Fatalf("#%d should replace the queued row:\n%s", n, v)
	}
	if b, _ := m.selectedBuild(); b.Number != n {
		t.Errorf("cursor on #%d, want the new #%d", b.Number, n)
	}
	if len(m.follows) != 0 {
		t.Errorf("follows = %v", m.follows)
	}
}

func TestRefreshKeepsTheCursorOnItsBuild(t *testing.T) {
	m, fj := liveModel(t)
	m = build(t, m)
	m = drive(t, m, key("j")) // #1: moved by hand, so no jump
	n := fj.start()
	m = drive(t, m, queueTickMsg{id: 9})
	if b, _ := m.selectedBuild(); b.Number != 1 {
		t.Errorf("cursor on #%d, want it to stay on #1 above the new #%d", b.Number, n)
	}
}

func TestRefreshCadence(t *testing.T) {
	m, fj := liveModel(t)
	if tickAt(m, 20).builds.polling {
		t.Error("nothing queued or running: no refresh")
	}

	fj.mu.Lock()
	fj.queued = true // queued by someone else
	fj.mu.Unlock()
	m = drive(t, m, key("r"))
	if tickAt(m, 30).builds.polling {
		t.Error("queued: refresh every 2s, not on frame 30")
	}
	if mm := tickAt(m, 40); !mm.builds.polling || mm.builds.loading {
		t.Errorf("queued: a quiet refresh on frame 40, polling=%v loading=%v", mm.builds.polling, mm.builds.loading)
	}

	m.builds.detail = &jenkins.Detail{Builds: []jenkins.Build{{Number: 3, Building: true}}}
	if tickAt(m, 40).builds.polling {
		t.Error("running: refresh every 5s, not on frame 40")
	}
	if !tickAt(m, 50).builds.polling {
		t.Error("running: refresh on frame 50")
	}
}

func TestQueuedBuildStartsElsewhere(t *testing.T) {
	m, fj := liveModel(t)
	m = build(t, m)
	m = drive(t, m, key("h")) // back to the jobs
	n := fj.start()
	m = drive(t, m, queueTickMsg{id: 9})
	if m.scr != scrJobs || !strings.Contains(m.status, fmt.Sprintf("lint #%d started", n)) {
		t.Errorf("screen=%d status=%q", m.scr, m.status)
	}
}

func TestQueuedBuildCancelled(t *testing.T) {
	m, fj := liveModel(t)
	m = build(t, m)
	fj.mu.Lock()
	fj.cancelled = true
	fj.mu.Unlock()
	m = drive(t, m, queueTickMsg{id: 9})
	if !m.statusErr || !strings.Contains(m.status, "cancelled") || len(m.follows) != 0 {
		t.Errorf("status=%q follows=%v", m.status, m.follows)
	}
}

func TestQueueErrorsAreSaidOnce(t *testing.T) {
	m, fj := liveModel(t)
	m = build(t, m)
	fj.mu.Lock()
	fj.failQueue = true
	fj.mu.Unlock()
	m = drive(t, m, queueTickMsg{id: 9})
	if !m.statusErr || m.follows[9] == nil {
		t.Fatalf("a failed check is reported and retried: status=%q follows=%v", m.status, m.follows)
	}
	m.setStatus("something else", false)
	m = drive(t, m, queueTickMsg{id: 9})
	if m.status != "something else" {
		t.Errorf("the same failure is said again: %q", m.status)
	}
}

func TestBackgroundRefreshErrorsAreSaidOnce(t *testing.T) {
	m, _ := liveModel(t)
	m.builds.polling = true
	next, _ := m.Update(detailMsg{job: m.builds.job, err: fmt.Errorf("boom")})
	m = next.(Model)
	if !strings.Contains(m.status, "boom") || m.builds.detail == nil {
		t.Fatalf("status=%q, the list should stay", m.status)
	}
	m.setStatus("something else", false)
	m.builds.polling = true
	next, _ = m.Update(detailMsg{job: m.builds.job, err: fmt.Errorf("boom")})
	if mm := next.(Model); mm.status != "something else" {
		t.Errorf("the same failure is said again: %q", mm.status)
	}
}

func TestRebuildFromLogSwitchesToTheNewLog(t *testing.T) {
	m, fj := liveModel(t)
	m = drive(t, m, key("enter")) // #2's log
	m = drive(t, m, key("R"))     // no parameters: just confirm
	m = drive(t, m, key("y"))
	if m.scr != scrLog || m.log.number != 2 {
		t.Fatalf("screen=%d log=#%d", m.scr, m.log.number)
	}
	n := fj.start()
	m = drive(t, m, queueTickMsg{id: 9})
	if m.scr != scrLog || m.log.number != n {
		t.Errorf("should follow the new build's log: screen=%d log=#%d", m.scr, m.log.number)
	}
}

func TestRebuildFromLogStaysOnAnotherLog(t *testing.T) {
	m, fj := liveModel(t)
	m = drive(t, m, key("enter")) // #2's log
	m = drive(t, m, key("R"))
	m = drive(t, m, key("y"))
	m = drive(t, m, key("h"))     // builds
	m = drive(t, m, key("j"))     // #1
	m = drive(t, m, key("enter")) // #1's log
	fj.start()
	m = drive(t, m, queueTickMsg{id: 9})
	if m.scr != scrLog || m.log.number != 1 {
		t.Errorf("moved on to #1: screen=%d log=#%d", m.scr, m.log.number)
	}
}

func TestQueueBacksOff(t *testing.T) {
	m := New(Options{Config: newCfg(t), NoAnim: true})
	if d := m.queueDelay(&follow{since: time.Now()}); d != 2*time.Second {
		t.Errorf("first minute: %v", d)
	}
	if d := m.queueDelay(&follow{since: time.Now().Add(-2 * time.Minute)}); d != time.Duration(m.cfg.PollSeconds)*time.Second {
		t.Errorf("stuck: %v, want the watch interval", d)
	}
}
