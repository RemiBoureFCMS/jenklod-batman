// Package update finds newer releases of jenklod-batman on GitHub and
// installs them the way the running copy was installed.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strconv"
	"time"
)

// Module is the Go module path, for go install.
const Module = "github.com/krank56/jenklod-batman"

const repo = "krank56/jenklod-batman"

// Version is a release version, vMAJOR.MINOR.PATCH.
type Version struct{ Major, Minor, Patch int }

var semver = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

// Parse reads a release version. Anything else (dev, or a git describe
// version such as v0.3.0-12-gc44f272) is a development build: false.
func Parse(s string) (Version, bool) {
	m := semver.FindStringSubmatch(s)
	if m == nil {
		return Version{}, false
	}
	n := func(i int) int { x, _ := strconv.Atoi(m[i]); return x }
	return Version{n(1), n(2), n(3)}, true
}

func (v Version) String() string { return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch) }

// Less reports whether v is older than w.
func (v Version) Less(w Version) bool {
	if v.Major != w.Major {
		return v.Major < w.Major
	}
	if v.Minor != w.Minor {
		return v.Minor < w.Minor
	}
	return v.Patch < w.Patch
}

// Current is the running version: the one stamped at build time (make,
// release archives, Homebrew), else the module version Go recorded for
// `go install …@vX.Y.Z`, in which case fromBuildInfo is true.
func Current(stamped string, bi *debug.BuildInfo) (v string, fromBuildInfo bool) {
	if stamped != "" && stamped != "dev" {
		return stamped, false
	}
	if bi != nil {
		if _, ok := Parse(bi.Main.Version); ok {
			return bi.Main.Version, true
		}
	}
	return "dev", false
}

// Release is a published GitHub release.
type Release struct {
	Tag       string
	Published time.Time
	Notes     string
}

// Source reads releases from GitHub. The repository is public: no token.
type Source struct {
	API      string // https://api.github.com/repos/<repo>
	Download string // https://github.com/<repo>/releases/download
	HTTP     *http.Client
}

// NewSource reads this project's releases.
func NewSource() *Source {
	return &Source{
		API:      "https://api.github.com/repos/" + repo,
		Download: "https://github.com/" + repo + "/releases/download",
		HTTP:     &http.Client{Timeout: 2 * time.Minute},
	}
}

type apiRelease struct {
	Tag        string    `json:"tag_name"`
	Published  time.Time `json:"published_at"`
	Body       string    `json:"body"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
}

func (a apiRelease) release() Release {
	return Release{Tag: a.Tag, Published: a.Published, Notes: a.Body}
}

// Latest is the newest published release.
func (s *Source) Latest(ctx context.Context) (Release, error) {
	var a apiRelease
	if err := s.getJSON(ctx, s.API+"/releases/latest", &a); err != nil {
		return Release{}, err
	}
	return a.release(), nil
}

// Releases lists the published releases, newest first.
func (s *Source) Releases(ctx context.Context) ([]Release, error) {
	var as []apiRelease
	if err := s.getJSON(ctx, s.API+"/releases?per_page=100", &as); err != nil {
		return nil, err
	}
	var out []Release
	for _, a := range as {
		if !a.Draft && !a.Prerelease {
			out = append(out, a.release())
		}
	}
	return out, nil
}

// Between keeps the releases newer than from, up to to, oldest first.
func Between(rels []Release, from, to Version) []Release {
	var out []Release
	for _, r := range rels {
		if v, ok := Parse(r.Tag); ok && from.Less(v) && !to.Less(v) {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b Release) int {
		va, _ := Parse(a.Tag)
		vb, _ := Parse(b.Tag)
		if va.Less(vb) {
			return -1
		}
		return 1
	})
	return out
}

func (s *Source) getJSON(ctx context.Context, url string, v any) error {
	b, err := s.get(ctx, url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// get fetches url, up to 200 MB.
func (s *Source) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "jenklod-batman")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

// CheckEvery is how long an answer from GitHub is kept.
const CheckEvery = 24 * time.Hour

// Settle is how old a release must be before it is announced: the
// Homebrew tap picks new releases up within the hour.
const Settle = time.Hour

// State is what the update check remembers between runs.
type State struct {
	Checked   time.Time `json:"checked,omitzero"`
	Latest    string    `json:"latest,omitempty"`
	Published time.Time `json:"published,omitzero"`
	Announced string    `json:"announced,omitempty"` // the release the TUI last told about
	Seen      string    `json:"seen,omitempty"`      // the version that last ran, for what's new

	path string
}

// StatePath is $XDG_CACHE_HOME (or ~/.cache)/jenklod-batman/update.json.
func StatePath() (string, error) {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "jenklod-batman", "update.json"), nil
}

// LoadState reads the state at path; a missing or unreadable file is an
// empty state, so the worst case is checking again.
func LoadState(path string) (*State, error) {
	s := &State{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return &State{path: path}, err
	}
	return s, nil
}

// Save writes the state.
func (s *State) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Due reports whether the cached answer is too old to use.
func (s *State) Due(now time.Time) bool { return now.Sub(s.Checked) >= CheckEvery }

// Record keeps GitHub's answer.
func (s *State) Record(r Release, now time.Time) {
	s.Checked, s.Latest, s.Published = now, r.Tag, r.Published
}

// Available is the cached latest release, if it is newer than current
// and has settled.
func (s *State) Available(current Version, now time.Time) (Version, bool) {
	v, ok := Parse(s.Latest)
	if !ok || !current.Less(v) || now.Before(s.Published.Add(Settle)) {
		return Version{}, false
	}
	return v, true
}

// Checker is the background check of the TUI.
type Checker struct {
	Source  *Source
	State   *State
	Current Version
	Now     func() time.Time
}

// Check asks GitHub if the cached answer is due, and returns the newer
// release to show, if any. news is true the first time a release is
// returned, so the TUI says it once.
func (c *Checker) Check(ctx context.Context) (v Version, news bool, err error) {
	now := c.Now()
	if c.State.Due(now) {
		r, err := c.Source.Latest(ctx)
		if err != nil {
			return Version{}, false, err
		}
		c.State.Record(r, now)
		if err := c.State.Save(); err != nil {
			return Version{}, false, err
		}
	}
	v, ok := c.State.Available(c.Current, now)
	if !ok {
		return Version{}, false, nil
	}
	if c.State.Announced == v.String() {
		return v, false, nil
	}
	c.State.Announced = v.String()
	return v, true, c.State.Save()
}
