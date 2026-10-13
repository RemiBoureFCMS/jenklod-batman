// Package github lists the branches of a GitHub repository, for the
// branch picker of the build form. It borrows the gh CLI's login: the token
// is asked of `gh auth token` for each fetch and never kept.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Repo is a repository on a GitHub host (github.com or an Enterprise one).
type Repo struct {
	Host, Owner, Name string
}

// String is owner/name on github.com, host/owner/name elsewhere; ParseRepo
// reads it back.
func (r Repo) String() string {
	if r.Host == "github.com" {
		return r.Owner + "/" + r.Name
	}
	return r.Host + "/" + r.Owner + "/" + r.Name
}

var (
	scpLike = regexp.MustCompile(`^[\w.-]+@([^:/]+):(.+)$`)
	segment = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// ParseRepo reads owner/name, host/owner/name, or a clone URL (https, ssh,
// or git@host:owner/name.git).
func ParseRepo(s string) (Repo, bool) {
	s = strings.TrimSpace(s)
	var host, path string
	switch {
	case strings.Contains(s, "://"):
		u, err := url.Parse(s)
		if err != nil {
			return Repo{}, false
		}
		host, path = u.Hostname(), u.Path
	case scpLike.MatchString(s):
		m := scpLike.FindStringSubmatch(s)
		host, path = m[1], m[2]
	default:
		path = s
	}
	parts := strings.Split(strings.TrimSuffix(strings.Trim(path, "/"), ".git"), "/")
	if host == "" && len(parts) == 3 && strings.Contains(parts[0], ".") {
		host, parts = parts[0], parts[1:]
	}
	if host == "" {
		host = "github.com"
	}
	if len(parts) != 2 || !segment.MatchString(parts[0]) || !segment.MatchString(parts[1]) {
		return Repo{}, false
	}
	return Repo{Host: strings.ToLower(host), Owner: parts[0], Name: parts[1]}, true
}

// Branch is one branch of a repository.
type Branch struct {
	Name      string
	Committed time.Time // of its last commit
	Default   bool
}

// MaxBranches caps how many branches are fetched.
const MaxBranches = 1000

var (
	// ErrNoGH means the gh CLI is not installed.
	ErrNoGH = errors.New("gh is not installed: install it, then run gh auth login")
	// ErrNotLoggedIn means gh has no token for the host.
	ErrNotLoggedIn = errors.New("gh is not logged in")
)

// Client talks to the GitHub GraphQL API.
type Client struct {
	API   func(host string) string          // the GraphQL endpoint of a host
	Token func(host string) (string, error) // a token for host; never stored
	HTTP  *http.Client
}

// New uses the real API and gh's login.
func New() *Client {
	return &Client{API: apiURL, Token: ghToken, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func apiURL(host string) string {
	if host == "github.com" {
		return "https://api.github.com/graphql"
	}
	return "https://" + host + "/api/graphql"
}

func ghToken(host string) (string, error) {
	out, err := exec.Command("gh", "auth", "token", "--hostname", host).Output()
	if errors.Is(err, exec.ErrNotFound) {
		return "", ErrNoGH
	}
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return "", fmt.Errorf("%w to %s: run gh auth login --hostname %s", ErrNotLoggedIn, host, host)
	}
	return string(bytes.TrimSpace(out)), nil
}

const branchesQuery = `query($owner: String!, $name: String!, $after: String) {
  repository(owner: $owner, name: $name) {
    defaultBranchRef { name }
    refs(refPrefix: "refs/heads/", first: 100, after: $after) {
      pageInfo { hasNextPage endCursor }
      nodes { name target { ... on Commit { committedDate } } }
    }
  }
}`

// Branches lists the branches of r: the default one first, then the most
// recently committed to.
func (c *Client) Branches(ctx context.Context, r Repo) ([]Branch, error) {
	token, err := c.Token(r.Host)
	if err != nil {
		return nil, err
	}
	var (
		out   []Branch
		def   string
		after any // null on the first page
	)
	for len(out) < MaxBranches {
		var page struct {
			Repository *struct {
				DefaultBranchRef *struct{ Name string } `json:"defaultBranchRef"`
				Refs             struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						Name   string `json:"name"`
						Target struct {
							CommittedDate time.Time `json:"committedDate"`
						} `json:"target"`
					} `json:"nodes"`
				} `json:"refs"`
			} `json:"repository"`
		}
		vars := map[string]any{"owner": r.Owner, "name": r.Name, "after": after}
		if err := c.query(ctx, r.Host, token, vars, &page); err != nil {
			return nil, err
		}
		repo := page.Repository
		if repo == nil {
			return nil, fmt.Errorf("%s: no such repository, or gh's account cannot see it", r)
		}
		if repo.DefaultBranchRef != nil {
			def = repo.DefaultBranchRef.Name
		}
		for _, n := range repo.Refs.Nodes {
			out = append(out, Branch{Name: n.Name, Committed: n.Target.CommittedDate})
		}
		if !repo.Refs.PageInfo.HasNextPage {
			break
		}
		after = repo.Refs.PageInfo.EndCursor
	}
	out = out[:min(len(out), MaxBranches)]
	for i := range out {
		out[i].Default = out[i].Name == def
	}
	slices.SortStableFunc(out, func(a, b Branch) int {
		switch {
		case a.Default != b.Default:
			if a.Default {
				return -1
			}
			return 1
		case !a.Committed.Equal(b.Committed):
			return b.Committed.Compare(a.Committed)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

// query runs one GraphQL query; errors never carry the token.
func (c *Client) query(ctx context.Context, host, token string, vars map[string]any, data any) error {
	body, err := json.Marshal(map[string]any{"query": branchesQuery, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.API(host), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("GitHub refused gh's token for %s: run gh auth login --hostname %s", host, host)
	case resp.StatusCode != http.StatusOK:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("GitHub: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("GitHub: %w", err)
	}
	if len(out.Errors) > 0 {
		var msgs []string
		for _, e := range out.Errors {
			msgs = append(msgs, e.Message)
		}
		return fmt.Errorf("GitHub: %s", strings.Join(msgs, "; "))
	}
	return json.Unmarshal(out.Data, data)
}
