package store

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type fakeCommit struct {
	tree, parent, message string
	author                map[string]string
}

// fakeGitHub implements the handful of REST endpoints the backend uses.
type fakeGitHub struct {
	mu          sync.Mutex
	trees       map[string]map[string]string
	commits     map[string]fakeCommit
	head        string
	n           int
	patches     int
	userStatus  int
	refRule     string                     // when set, PATCH ref is refused with this 422 message and head stays put
	beforePatch func(f *fakeGitHub, n int) // n counts earlier PATCHes; runs without the lock held
}

func newFake(t *testing.T, files map[string]string) (*fakeGitHub, *GitHub) {
	t.Helper()
	f := &fakeGitHub{
		trees:      map[string]map[string]string{"t0": files},
		commits:    map[string]fakeCommit{"c0": {tree: "t0"}},
		head:       "c0",
		userStatus: http.StatusOK,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user", f.user)
	mux.HandleFunc("GET /repos/o/r/git/ref/heads/main", f.getRef)
	mux.HandleFunc("GET /repos/o/r/git/commits/{sha}", f.getCommit)
	mux.HandleFunc("GET /repos/o/r/tarball/{sha}", f.tarball)
	mux.HandleFunc("POST /repos/o/r/git/trees", f.postTree)
	mux.HandleFunc("POST /repos/o/r/git/commits", f.postCommit)
	mux.HandleFunc("PATCH /repos/o/r/git/refs/heads/main", f.patchRef)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	g, err := NewGitHub("o/r", "", "tok")
	if err != nil {
		t.Fatal(err)
	}
	g.BaseURL, g.Client, g.Sleep = srv.URL, srv.Client(), func(time.Duration) {}
	return f, g
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeGitHub) user(w http.ResponseWriter, _ *http.Request) {
	if f.userStatus != http.StatusOK {
		writeJSON(w, f.userStatus, map[string]string{"message": "Resource not accessible by integration"})
		return
	}
	writeJSON(w, 200, map[string]any{"id": 7, "login": "octo"})
}

func (f *fakeGitHub) getRef(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	writeJSON(w, 200, map[string]any{"object": map[string]string{"sha": f.head}})
}

func (f *fakeGitHub) getCommit(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.commits[r.PathValue("sha")]
	if !ok {
		writeJSON(w, 404, map[string]string{"message": "Not Found"})
		return
	}
	writeJSON(w, 200, map[string]any{"sha": r.PathValue("sha"), "tree": map[string]string{"sha": c.tree}})
}

func (f *fakeGitHub) tarball(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	sha := r.PathValue("sha")
	files := maps.Clone(f.trees[f.commits[sha].tree])
	f.mu.Unlock()
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	prefix := "o-r-" + sha + "/"
	_ = tw.WriteHeader(&tar.Header{Name: prefix, Typeflag: tar.TypeDir, Mode: 0o755})
	for name, body := range files {
		_ = tw.WriteHeader(&tar.Header{Name: prefix + name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
}

func (f *fakeGitHub) postTree(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BaseTree string `json:"base_tree"`
		Tree     []struct{ Path, Mode, Type, Content string }
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	defer f.mu.Unlock()
	files := maps.Clone(f.trees[body.BaseTree])
	for _, e := range body.Tree {
		files[e.Path] = e.Content
	}
	f.n++
	id := fmt.Sprintf("t%d", f.n)
	f.trees[id] = files
	writeJSON(w, 201, map[string]string{"sha": id})
}

func (f *fakeGitHub) postCommit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string
		Tree    string
		Parents []string
		Author  map[string]string
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	id := fmt.Sprintf("c%d", f.n)
	f.commits[id] = fakeCommit{tree: body.Tree, parent: body.Parents[0], message: body.Message, author: body.Author}
	writeJSON(w, 201, map[string]string{"sha": id})
}

func (f *fakeGitHub) patchRef(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SHA   string `json:"sha"`
		Force bool   `json:"force"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	n := f.patches
	f.patches++
	f.mu.Unlock()
	if f.beforePatch != nil {
		f.beforePatch(f, n)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refRule != "" {
		writeJSON(w, 422, map[string]string{"message": f.refRule})
		return
	}
	if body.Force {
		writeJSON(w, 400, map[string]string{"message": "test: force must never be used"})
		return
	}
	if f.commits[body.SHA].parent != f.head {
		writeJSON(w, 422, map[string]string{"message": "Update is not a fast forward"})
		return
	}
	f.head = body.SHA
	writeJSON(w, 200, map[string]any{"object": map[string]string{"sha": f.head}})
}

// pushDirect simulates someone else committing files on top of head.
func (f *fakeGitHub) pushDirect(files map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tree := maps.Clone(f.trees[f.commits[f.head].tree])
	maps.Copy(tree, files)
	f.n++
	tid, cid := fmt.Sprintf("t%d", f.n), fmt.Sprintf("c%d", f.n)
	f.trees[tid] = tree
	f.commits[cid] = fakeCommit{tree: tid, parent: f.head, message: "concurrent"}
	f.head = cid
}

// Accessors lock, so tests stay clean under -race.
func (f *fakeGitHub) headFiles() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.trees[f.commits[f.head].tree])
}

func (f *fakeGitHub) headCommit() (string, fakeCommit) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.head, f.commits[f.head]
}

func (f *fakeGitHub) patchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.patches
}
