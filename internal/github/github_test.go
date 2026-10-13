package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseRepo(t *testing.T) {
	for in, want := range map[string]Repo{
		"wayne/gotham":                            {"github.com", "wayne", "gotham"},
		" wayne/gotham ":                          {"github.com", "wayne", "gotham"},
		"https://github.com/wayne/gotham":         {"github.com", "wayne", "gotham"},
		"https://github.com/wayne/gotham.git":     {"github.com", "wayne", "gotham"},
		"https://github.com/wayne/gotham/":        {"github.com", "wayne", "gotham"},
		"https://jenkins@github.com/wayne/gotham": {"github.com", "wayne", "gotham"},
		"git@github.com:wayne/gotham.git":         {"github.com", "wayne", "gotham"},
		"ssh://git@github.com/wayne/gotham.git":   {"github.com", "wayne", "gotham"},
		"https://ghe.wayne.corp/cave/signal.git":  {"ghe.wayne.corp", "cave", "signal"},
	} {
		got, ok := ParseRepo(in)
		if !ok || got != want {
			t.Errorf("ParseRepo(%q) = %+v, %v; want %+v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "gotham", "wayne/gotham/extra", "https://github.com/wayne", "not a url/at all"} {
		if got, ok := ParseRepo(in); ok {
			t.Errorf("ParseRepo(%q) = %+v, want no repo", in, got)
		}
	}
	if s := (Repo{"github.com", "wayne", "gotham"}).String(); s != "wayne/gotham" {
		t.Errorf("String = %q", s)
	}
	if s := (Repo{"ghe.wayne.corp", "cave", "signal"}).String(); s != "ghe.wayne.corp/cave/signal" {
		t.Errorf("String = %q", s)
	}
	if r, ok := ParseRepo("ghe.wayne.corp/cave/signal"); !ok || r.Host != "ghe.wayne.corp" {
		t.Errorf("String must parse back: %+v %v", r, ok)
	}
}

// fakeGitHub serves the branches query from names, newest commit first
// unless dates says otherwise, 2 per page.
func fakeGitHub(t *testing.T, names []string, dates map[string]time.Time) (*Client, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("Authorization"); got != "bearer gh-test-value" {
			t.Errorf("authorization = %q", got)
		}
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Variables["owner"] != "wayne" || req.Variables["name"] != "gotham" {
			t.Errorf("variables = %v", req.Variables)
		}
		start := 0
		if after, _ := req.Variables["after"].(string); after != "" {
			fmt.Sscan(after, &start)
		}
		end := min(start+2, len(names))
		var nodes []string
		for _, n := range names[start:end] {
			d, ok := dates[n]
			if !ok {
				d = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			nodes = append(nodes, fmt.Sprintf(`{"name":%q,"target":{"committedDate":%q}}`, n, d.Format(time.RFC3339)))
		}
		fmt.Fprintf(w, `{"data":{"repository":{"defaultBranchRef":{"name":"main"},"refs":{
			"pageInfo":{"hasNextPage":%v,"endCursor":"%d"},"nodes":[%s]}}}}`,
			end < len(names), end, strings.Join(nodes, ","))
	}))
	t.Cleanup(srv.Close)
	return &Client{
		API:   func(host string) string { return srv.URL },
		Token: func(host string) (string, error) { return "gh-test-value", nil },
		HTTP:  srv.Client(),
	}, &calls
}

func TestBranchesDefaultFirstThenNewest(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }
	c, calls := fakeGitHub(t, []string{"feat/a", "main", "fix/b", "old", "feat/c"},
		map[string]time.Time{"feat/a": day(3), "main": day(1), "fix/b": day(9), "old": day(1), "feat/c": day(5)})
	got, err := c.Branches(context.Background(), Repo{"github.com", "wayne", "gotham"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range got {
		names = append(names, b.Name)
	}
	if want := "main fix/b feat/c feat/a old"; strings.Join(names, " ") != want {
		t.Errorf("order = %v, want %s", names, want)
	}
	if !got[0].Default || got[1].Default || !got[1].Committed.Equal(day(9)) {
		t.Errorf("branches = %+v", got[:2])
	}
	if *calls != 3 {
		t.Errorf("pages fetched = %d, want 3", *calls)
	}
}

func TestBranchesCapped(t *testing.T) {
	names := make([]string, MaxBranches+10)
	for i := range names {
		names[i] = fmt.Sprintf("b%04d", i)
	}
	c, _ := fakeGitHub(t, names, nil)
	got, err := c.Branches(context.Background(), Repo{"github.com", "wayne", "gotham"})
	if err != nil || len(got) != MaxBranches {
		t.Errorf("got %d branches, %v; want %d", len(got), err, MaxBranches)
	}
}

func TestBranchesErrors(t *testing.T) {
	c, calls := fakeGitHub(t, []string{"main"}, nil)
	c.Token = func(host string) (string, error) { return "", ErrNoGH }
	if _, err := c.Branches(context.Background(), Repo{"github.com", "wayne", "gotham"}); !errors.Is(err, ErrNoGH) || *calls != 0 {
		t.Errorf("err = %v, calls = %d", err, *calls)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"repository":null},"errors":[{"type":"NOT_FOUND","message":"Could not resolve to a Repository with the name 'wayne/gotham'."}]}`))
	}))
	defer srv.Close()
	c = &Client{API: func(string) string { return srv.URL }, Token: func(string) (string, error) { return "gh-test-value", nil }, HTTP: srv.Client()}
	_, err := c.Branches(context.Background(), Repo{"github.com", "wayne", "gotham"})
	if err == nil || !strings.Contains(err.Error(), "Could not resolve") || strings.Contains(err.Error(), "gh-test-value") {
		t.Errorf("err = %v", err)
	}

	srv401 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
	}))
	defer srv401.Close()
	c.API = func(string) string { return srv401.URL }
	c.HTTP = srv401.Client()
	if _, err := c.Branches(context.Background(), Repo{"github.com", "wayne", "gotham"}); err == nil || !strings.Contains(err.Error(), "gh auth login") {
		t.Errorf("401 should say how to log in: %v", err)
	}
}

func TestAPIEndpoint(t *testing.T) {
	if got := apiURL("github.com"); got != "https://api.github.com/graphql" {
		t.Errorf("github.com: %s", got)
	}
	if got := apiURL("ghe.wayne.corp"); got != "https://ghe.wayne.corp/api/graphql" {
		t.Errorf("enterprise: %s", got)
	}
}

func TestAPIFromEnvSendsNoToken(t *testing.T) {
	var auth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		w.Write([]byte(`{"data":{"repository":{"defaultBranchRef":{"name":"main"},"refs":{"pageInfo":{"hasNextPage":false},"nodes":[{"name":"main","target":{}}]}}}}`))
	}))
	defer srv.Close()
	t.Setenv(APIEnv, srv.URL)
	c := New()
	c.HTTP = srv.Client()
	got, err := c.Branches(context.Background(), Repo{"github.com", "wayne", "gotham"})
	if err != nil || len(got) != 1 || len(auth) != 1 || auth[0] != "" {
		t.Errorf("branches=%v err=%v auth=%q: gh must not be asked, nor any token sent", got, err, auth)
	}
}
