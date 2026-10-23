package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Fab-IT-ApS/statusctl/model"
	"github.com/Fab-IT-ApS/statusctl/ops"
	"github.com/Fab-IT-ApS/statusctl/store"
)

type fileConfig struct {
	DefaultProfile string             `yaml:"default_profile"`
	Profiles       map[string]profile `yaml:"profiles"`
}

// profile deliberately has no token field: tokens come from the environment
// or token_cmd, never from a plain-text file.
type profile struct {
	Repo     string `yaml:"repo"`
	Branch   string `yaml:"branch"`
	TokenCmd string `yaml:"token_cmd"`
	Author   string `yaml:"author"`
}

func inputErrorf(format string, args ...any) error {
	return &ops.InputError{Msg: fmt.Sprintf(format, args...)}
}

func (a *app) configFile() (string, error) {
	if a.configPath != "" {
		return a.configPath, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "statusctl", "config.yaml"), nil
}

func (a *app) loadProfile() (string, profile, error) {
	path, err := a.configFile()
	if err != nil {
		return "", profile{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", profile{}, inputErrorf("no config file at %s; create one or pass --dir", path)
	}
	if err != nil {
		return "", profile{}, err
	}
	var fc fileConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&fc); err != nil {
		return "", profile{}, inputErrorf("%s: %v", path, err)
	}
	name := a.profile
	if name == "" {
		name = a.getenv("STATUSCTL_PROFILE")
	}
	if name == "" {
		name = fc.DefaultProfile
	}
	if name == "" {
		return "", profile{}, inputErrorf("no profile selected; pass --profile, set STATUSCTL_PROFILE or set default_profile in %s", path)
	}
	p, ok := fc.Profiles[name]
	if !ok {
		return "", profile{}, inputErrorf("profile %q not found in %s", name, path)
	}
	if p.Branch == "" {
		p.Branch = "main"
	}
	return name, p, nil
}

func (a *app) token(p profile) (string, error) {
	if t := a.getenv("STATUSCTL_GITHUB_TOKEN"); t != "" {
		return t, nil
	}
	if p.TokenCmd == "" {
		return "", inputErrorf("no GitHub token: set STATUSCTL_GITHUB_TOKEN or token_cmd in the profile")
	}
	out, err := a.runShell(p.TokenCmd)
	if err != nil {
		return "", fmt.Errorf("%w: token_cmd failed: %v", store.ErrAuth, err)
	}
	t := strings.TrimSpace(out)
	if t == "" {
		return "", fmt.Errorf("%w: token_cmd printed nothing", store.ErrAuth)
	}
	return t, nil
}

// openStore returns the local store for --dir, otherwise the GitHub store
// from the selected profile.
func (a *app) openStore() (store.Store, profile, error) {
	if a.dir != "" {
		return store.Local{Dir: a.dir}, profile{}, nil
	}
	name, p, err := a.loadProfile()
	if err != nil {
		return nil, profile{}, err
	}
	tok, err := a.token(p)
	if err != nil {
		return nil, profile{}, err
	}
	gh, err := store.NewGitHub(p.Repo, p.Branch, tok)
	if err != nil {
		return nil, profile{}, err
	}
	return profileStore{Store: gh, name: name}, p, nil
}

// profileStore adds the profile name to auth and network errors, so a user
// with several organisations can tell which token or repo to fix.
type profileStore struct {
	store.Store
	name string
}

func (s profileStore) Load(ctx context.Context) (*model.State, error) {
	st, err := s.Store.Load(ctx)
	return st, s.wrap(err)
}

func (s profileStore) Apply(ctx context.Context, op store.Op) (*ops.Result, error) {
	res, err := s.Store.Apply(ctx, op)
	return res, s.wrap(err)
}

func (s profileStore) wrap(err error) error {
	if errors.Is(err, store.ErrAuth) || errors.Is(err, store.ErrNetwork) {
		return fmt.Errorf("profile %q: %w", s.name, err)
	}
	return err
}
