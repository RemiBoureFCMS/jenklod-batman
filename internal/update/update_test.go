package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func TestParseAndCompare(t *testing.T) {
	for in, want := range map[string]Version{"v0.3.0": {0, 3, 0}, "0.10.2": {0, 10, 2}, "v1.0.0": {1, 0, 0}} {
		if got, ok := Parse(in); !ok || got != want {
			t.Errorf("Parse(%q) = %v, %v", in, got, ok)
		}
	}
	for _, in := range []string{"dev", "", "v0.3", "v0.3.0-12-gc44f272", "v0.3.0-dirty", "(devel)", "v0.3.0+meta"} {
		if v, ok := Parse(in); ok {
			t.Errorf("Parse(%q) = %v: not a release", in, v)
		}
	}
	a, _ := Parse("v0.9.9")
	b, _ := Parse("v0.10.0")
	if !a.Less(b) || b.Less(a) || a.Less(a) {
		t.Error("Less is not numeric")
	}
	if b.String() != "v0.10.0" {
		t.Errorf("String = %s", b)
	}
}

func TestCurrent(t *testing.T) {
	bi := &debug.BuildInfo{Main: debug.Module{Path: Module, Version: "v0.4.0"}}
	if v, fromBI := Current("v0.3.0", bi); v != "v0.3.0" || fromBI {
		t.Errorf("stamped wins: %s %v", v, fromBI)
	}
	if v, fromBI := Current("dev", bi); v != "v0.4.0" || !fromBI {
		t.Errorf("go install: %s %v", v, fromBI)
	}
	if v, fromBI := Current("dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}); v != "dev" || fromBI {
		t.Errorf("go build: %s %v", v, fromBI)
	}
	if v, _ := Current("dev", nil); v != "dev" {
		t.Errorf("no build info: %s", v)
	}
}

func TestDetect(t *testing.T) {
	for _, c := range []struct {
		exe    string
		fromBI bool
		want   Method
	}{
		{"/opt/homebrew/Cellar/jenklod-batman/0.3.0/bin/jenklod-batman", false, Homebrew},
		{"/home/linuxbrew/.linuxbrew/Cellar/jenklod-batman/0.3.0/bin/jenklod-batman", false, Homebrew},
		{"/Users/bruce/go/bin/jenklod-batman", true, GoInstall},
		{"/usr/local/bin/jenklod-batman", false, Archive},
	} {
		if got := Detect(c.exe, c.fromBI); got != c.want {
			t.Errorf("Detect(%s, %v) = %v, want %v", c.exe, c.fromBI, got, c.want)
		}
	}
}

// fakeGitHub serves releases (newest first) and their assets.
type fakeGitHub struct {
	releases []string // tag|published RFC3339|notes
	assets   map[string][]byte
	calls    int
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls++
	rel := func(s string) string {
		p := strings.SplitN(s, "|", 3)
		return fmt.Sprintf(`{"tag_name":%q,"published_at":%q,"body":%q,"draft":false,"prerelease":false}`, p[0], p[1], p[2])
	}
	switch {
	case r.URL.Path == "/api/releases/latest":
		fmt.Fprint(w, rel(f.releases[0]))
	case r.URL.Path == "/api/releases":
		var all []string
		for _, s := range f.releases {
			all = append(all, rel(s))
		}
		all = append(all, `{"tag_name":"v9.9.9","draft":false,"prerelease":true}`)
		fmt.Fprintf(w, "[%s]", strings.Join(all, ","))
	case strings.HasPrefix(r.URL.Path, "/download/"):
		b, ok := f.assets[strings.TrimPrefix(r.URL.Path, "/download/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	default:
		http.NotFound(w, r)
	}
}

func newFake(t *testing.T, f *fakeGitHub) *Source {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return &Source{API: srv.URL + "/api", Download: srv.URL + "/download", HTTP: srv.Client()}
}

func TestCheckCachesAndSettles(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	f := &fakeGitHub{releases: []string{"v0.4.0|2026-10-14T11:30:00Z|notes"}}
	st, _ := LoadState(filepath.Join(t.TempDir(), "update.json"))
	cur, _ := Parse("v0.3.0")
	c := &Checker{Source: newFake(t, f), State: st, Current: cur, Now: func() time.Time { return now }}

	// Out for 30 minutes: Homebrew may not have it yet.
	if v, news, err := c.Check(context.Background()); err != nil || v != (Version{}) || news {
		t.Fatalf("too fresh: %v %v %v", v, news, err)
	}
	now = now.Add(time.Hour)
	v, news, err := c.Check(context.Background())
	if err != nil || v.String() != "v0.4.0" || !news {
		t.Fatalf("after an hour: %v %v %v", v, news, err)
	}
	if f.calls != 1 {
		t.Errorf("checked GitHub %d times within a day", f.calls)
	}
	again, _ := LoadState(st.path)
	if again.Latest != "v0.4.0" || again.Announced != "v0.4.0" {
		t.Errorf("state not saved: %+v", again)
	}
	c.State = again
	if v, news, _ := c.Check(context.Background()); v.String() != "v0.4.0" || news {
		t.Errorf("announced once only: %v %v", v, news)
	}
	now = now.Add(25 * time.Hour)
	c.Check(context.Background())
	if f.calls != 2 {
		t.Errorf("a day later it checks again: %d calls", f.calls)
	}
	c.Current, _ = Parse("v0.4.0")
	if v, _, _ := c.Check(context.Background()); v != (Version{}) {
		t.Errorf("up to date: %v", v)
	}
}

func TestNotesBetween(t *testing.T) {
	f := &fakeGitHub{releases: []string{
		"v0.5.0|2026-10-20T00:00:00Z|five", "v0.4.0|2026-10-14T00:00:00Z|four", "v0.3.0|2026-09-29T00:00:00Z|three"}}
	s := newFake(t, f)
	rels, err := s.Releases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	from, _ := Parse("v0.3.0")
	to, _ := Parse("v0.5.0")
	var got []string
	for _, r := range Between(rels, from, to) {
		got = append(got, r.Tag+":"+r.Notes)
	}
	if strings.Join(got, " ") != "v0.4.0:four v0.5.0:five" {
		t.Errorf("between = %v", got)
	}
}

func tarball(t *testing.T, dir string, bin []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string][]byte{dir + "/README.md": []byte("readme"), dir + "/jenklod-batman": bin} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))})
		tw.Write(body)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func archiveFake(t *testing.T, bin []byte, tamper bool) (*Installer, string) {
	t.Helper()
	name := "jenklod-batman_v0.4.0_linux_arm64"
	tgz := tarball(t, name, bin)
	sum := sha256.Sum256(tgz)
	if tamper {
		sum[0]++
	}
	f := &fakeGitHub{assets: map[string][]byte{
		"v0.4.0/" + name + ".tar.gz": tgz,
		"v0.4.0/checksums.txt":       fmt.Appendf(nil, "%x  other.tar.gz\n%x  %s.tar.gz\n", [32]byte{}, sum, name),
	}}
	exe := filepath.Join(t.TempDir(), "jenklod-batman")
	os.WriteFile(exe, []byte("old binary"), 0o755)
	return &Installer{Source: newFake(t, f), Exe: exe, GOOS: "linux", GOARCH: "arm64"}, exe
}

func TestInstallArchive(t *testing.T) {
	in, exe := archiveFake(t, []byte("new binary"), false)
	to, _ := Parse("v0.4.0")
	if err := in.Install(context.Background(), Archive, to); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(exe)
	fi, _ := os.Stat(exe)
	if string(b) != "new binary" || fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("binary = %q, mode %v", b, fi.Mode())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".jenklod-batman-*")); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func TestInstallArchiveRefusesABadChecksum(t *testing.T) {
	in, exe := archiveFake(t, []byte("new binary"), true)
	to, _ := Parse("v0.4.0")
	err := in.Install(context.Background(), Archive, to)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old binary" {
		t.Errorf("binary replaced anyway: %q", b)
	}
}

func TestInstallArchiveWithoutABuildForThisPlatform(t *testing.T) {
	in, _ := archiveFake(t, []byte("new binary"), false)
	in.GOOS = "windows"
	to, _ := Parse("v0.4.0")
	if err := in.Install(context.Background(), Archive, to); err == nil || !strings.Contains(err.Error(), "windows/arm64") {
		t.Errorf("err = %v", err)
	}
}

func TestInstallWithBrewAndGo(t *testing.T) {
	var ran []string
	brewHas := "0.4.0"
	in := &Installer{
		Run: func(name string, args ...string) error {
			ran = append(ran, name+" "+strings.Join(args, " "))
			return nil
		},
		Output: func(name string, args ...string) ([]byte, error) {
			return []byte("jenklod-batman " + brewHas + "\n"), nil
		},
	}
	to, _ := Parse("v0.4.0")
	if err := in.Install(context.Background(), Homebrew, to); err != nil {
		t.Fatal(err)
	}
	if err := in.Install(context.Background(), GoInstall, to); err != nil {
		t.Fatal(err)
	}
	want := "brew upgrade krank56/tap/jenklod-batman | go install github.com/krank56/jenklod-batman@v0.4.0"
	if got := strings.Join(ran, " | "); got != want {
		t.Errorf("ran %q, want %q", got, want)
	}

	brewHas = "0.3.0"
	if err := in.Install(context.Background(), Homebrew, to); !errors.Is(err, ErrTapBehind) {
		t.Errorf("tap behind: %v", err)
	}
}
