package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing/fstest"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
	"github.com/Fab-IT-ApS/statusctl/ops"
)

const maxTarball = 64 << 20

var errUnprocessable = errors.New("GitHub rejected the request")

// GitHub stores state in a GitHub repository, writing through the REST API
// with a compare-and-swap ref update.
type GitHub struct {
	BaseURL  string
	Owner    string
	Repo     string
	Branch   string
	Token    string
	Client   *http.Client
	Attempts int
	Sleep    func(time.Duration)
}

func NewGitHub(repo, branch, token string) (*GitHub, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return nil, &ops.InputError{Msg: fmt.Sprintf("repo %q must look like owner/name", repo)}
	}
	if branch == "" {
		branch = "main"
	}
	return &GitHub{
		BaseURL: "https://api.github.com", Owner: owner, Repo: name, Branch: branch, Token: token,
		Client: &http.Client{Timeout: 30 * time.Second}, Attempts: 5, Sleep: time.Sleep,
	}, nil
}

type snapshot struct {
	head, tree string
	state      *model.State
}

func (g *GitHub) Load(ctx context.Context) (*model.State, error) {
	snap, err := g.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return snap.state, nil
}

func (g *GitHub) Apply(ctx context.Context, op Op) (*ops.Result, error) {
	email := g.authorEmail(ctx)
	for attempt := 1; attempt <= g.Attempts; attempt++ {
		snap, err := g.snapshot(ctx)
		if err != nil {
			return nil, err
		}
		res, err := op(snap.state)
		if err != nil {
			return nil, err
		}
		commit, err := g.commit(ctx, snap, res, email)
		if err != nil {
			return nil, err
		}
		err = g.call(ctx, http.MethodPatch, g.repoPath("git/refs/heads/"+g.Branch),
			map[string]any{"sha": commit, "force": false}, nil)
		if err == nil {
			return res, nil
		}
		if !errors.Is(err, errUnprocessable) {
			return nil, err
		}
		// A refusal with the head unchanged is branch protection or a
		// ruleset, not a race; retrying cannot help.
		if head, herr := g.head(ctx); herr == nil && head == snap.head {
			return nil, fmt.Errorf("%w: GitHub refused to update %s in %s/%s: %v",
				ErrAuth, g.Branch, g.Owner, g.Repo, strings.TrimPrefix(err.Error(), errUnprocessable.Error()+": "))
		}
		if attempt < g.Attempts {
			g.Sleep(backoff(attempt))
		}
	}
	return nil, ErrConflict
}

func backoff(attempt int) time.Duration {
	return time.Duration(attempt)*200*time.Millisecond + time.Duration(rand.Int64N(int64(200*time.Millisecond)))
}

func (g *GitHub) repoPath(p string) string { return "/repos/" + g.Owner + "/" + g.Repo + "/" + p }

func (g *GitHub) head(ctx context.Context) (string, error) {
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := g.call(ctx, http.MethodGet, g.repoPath("git/ref/heads/"+g.Branch), nil, &ref); err != nil {
		return "", err
	}
	return ref.Object.SHA, nil
}

func (g *GitHub) snapshot(ctx context.Context) (*snapshot, error) {
	head, err := g.head(ctx)
	if err != nil {
		return nil, err
	}
	var commit struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err := g.call(ctx, http.MethodGet, g.repoPath("git/commits/"+head), nil, &commit); err != nil {
		return nil, err
	}
	fsys, err := g.tarball(ctx, head)
	if err != nil {
		return nil, err
	}
	state, err := model.Load(fsys)
	if err != nil {
		return nil, err
	}
	return &snapshot{head: head, tree: commit.Tree.SHA, state: state}, nil
}

func (g *GitHub) tarball(ctx context.Context, sha string) (fs.FS, error) {
	resp, err := g.send(ctx, http.MethodGet, g.repoPath("tarball/"+sha), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	gz, err := gzip.NewReader(io.LimitReader(resp.Body, maxTarball))
	if err != nil {
		return nil, fmt.Errorf("%w: reading repository archive: %v", ErrNetwork, err)
	}
	tr := tar.NewReader(gz)
	fsys := fstest.MapFS{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: reading repository archive: %v", ErrNetwork, err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		_, name, ok := strings.Cut(h.Name, "/") // strip GitHub's "<owner>-<repo>-<sha>/" prefix
		if !ok || name == "" {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("%w: reading repository archive: %v", ErrNetwork, err)
		}
		fsys[name] = &fstest.MapFile{Data: data, Mode: 0o644}
	}
	return fsys, nil
}

func (g *GitHub) commit(ctx context.Context, snap *snapshot, res *ops.Result, email string) (string, error) {
	type entry struct {
		Path    string `json:"path"`
		Mode    string `json:"mode"`
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	var entries []entry
	for _, p := range slices.Sorted(maps.Keys(res.Files)) {
		entries = append(entries, entry{Path: p, Mode: "100644", Type: "blob", Content: string(res.Files[p])})
	}
	var tree struct {
		SHA string `json:"sha"`
	}
	if err := g.call(ctx, http.MethodPost, g.repoPath("git/trees"),
		map[string]any{"base_tree": snap.tree, "tree": entries}, &tree); err != nil {
		return "", err
	}
	body := map[string]any{"message": res.CommitMessage(), "tree": tree.SHA, "parents": []string{snap.head}}
	if email != "" {
		body["author"] = map[string]string{"name": res.Incident.Latest().Author, "email": email}
	}
	var c struct {
		SHA string `json:"sha"`
	}
	if err := g.call(ctx, http.MethodPost, g.repoPath("git/commits"), body, &c); err != nil {
		return "", err
	}
	return c.SHA, nil
}

// authorEmail is the token owner's noreply address, or "" when the token
// cannot read /user (e.g. a GitHub App installation token).
func (g *GitHub) authorEmail(ctx context.Context) string {
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := g.call(ctx, http.MethodGet, "/user", nil, &u); err != nil || u.Login == "" {
		return ""
	}
	return fmt.Sprintf("%d+%s@users.noreply.github.com", u.ID, u.Login)
}

func (g *GitHub) call(ctx context.Context, method, p string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	resp, err := g.send(ctx, method, p, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decoding GitHub response for %s: %v", ErrNetwork, p, err)
	}
	return nil
}

func (g *GitHub) send(ctx context.Context, method, p string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, g.BaseURL+p, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		// Redirected downloads carry a short-lived token in the query string.
		var ue *url.Error
		if errors.As(err, &ue) {
			if u, perr := url.Parse(ue.URL); perr == nil {
				u.RawQuery = ""
				ue.URL = u.String()
			} else {
				ue.URL = "(unparseable URL)"
			}
		}
		return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	if resp.StatusCode < 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	switch resp.StatusCode {
	case http.StatusNotFound:
		if strings.Contains(p, "/git/ref/heads/") {
			return nil, fmt.Errorf("%w: GitHub returned %s for branch %q in %s/%s; check the branch name and that the token can write to that repository",
				ErrAuth, resp.Status, g.Branch, g.Owner, g.Repo)
		}
		fallthrough
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("%w: GitHub returned %s for %s/%s; check that the token can write to that repository",
			ErrAuth, resp.Status, g.Owner, g.Repo)
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("%w: %s", errUnprocessable, bytes.TrimSpace(msg))
	}
	return nil, fmt.Errorf("%w: GitHub returned %s for %s: %s", ErrNetwork, resp.Status, p, bytes.TrimSpace(msg))
}
