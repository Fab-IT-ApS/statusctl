package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Fab-IT-ApS/statusctl/model"
)

type msgFlags struct{ text, file string }

func (m *msgFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&m.text, "message", "m", "", "message text (Markdown)")
	cmd.Flags().StringVarP(&m.file, "file", "F", "", "read the message from FILE, or - for stdin")
}

// message returns the message from -m, -F or $EDITOR. When required is false
// and none is given it returns "" so the operation can use its default.
func (a *app) message(m msgFlags, required bool) (string, error) {
	switch {
	case m.text != "" && m.file != "":
		return "", inputErrorf("use either -m or -F, not both")
	case m.text != "":
		return m.text, nil
	case m.file == "-":
		b, err := io.ReadAll(a.stdin)
		return string(b), err
	case m.file != "":
		b, err := os.ReadFile(m.file)
		if err != nil {
			return "", inputErrorf("%v", err)
		}
		return string(b), nil
	case !required:
		return "", nil
	case !a.interactive:
		return "", inputErrorf("a message is required: pass -m TEXT, -F FILE or -F -")
	}
	return a.editMessage()
}

const editorTemplate = `
# Write the update above. Markdown is allowed.
# These two comment lines are removed; an empty message aborts.
`

func (a *app) editMessage() (string, error) {
	f, err := os.CreateTemp("", "statusctl-*.md")
	if err != nil {
		return "", err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.WriteString(editorTemplate); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	editor := a.getenv("VISUAL")
	if editor == "" {
		editor = a.getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	if err := a.runEditor(editor, name); err != nil {
		return "", fmt.Errorf("editor %q failed: %w", editor, err)
	}
	b, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	template := map[string]bool{}
	for _, l := range strings.Split(editorTemplate, "\n") {
		if strings.HasPrefix(l, "#") {
			template[l] = true
		}
	}
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if !template[strings.TrimRight(l, "\r")] {
			lines = append(lines, l)
		}
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		return "", inputErrorf("empty message; nothing was posted")
	}
	a.draft = text
	return text, nil
}

// saveDraft keeps a message typed in the editor when the post fails, so the
// user can fix the command and reuse it with -F.
func (a *app) saveDraft() {
	if a.draft == "" {
		return
	}
	f, err := os.CreateTemp("", "statusctl-draft-*.md")
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.WriteString(a.draft + "\n"); err != nil {
		return
	}
	fmt.Fprintf(a.stderr, "statusctl: your message saved to %s (reuse it with -F %s)\n", f.Name(), f.Name())
}

func runEditor(editor, file string) error {
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.Command("cmd", "/C", editor, file)
	} else {
		c = exec.Command("sh", "-c", editor+` "$1"`, "sh", file)
	}
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func gitUserName() string {
	out, err := exec.Command("git", "config", "user.name").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// parseImpacts turns repeated -c ID=IMPACT flags into a map. Whether the IDs
// and impacts are valid is checked by ops against the repository's config.
func parseImpacts(vals []string) (map[string]model.Impact, error) {
	out := map[string]model.Impact{}
	for _, v := range vals {
		id, imp, ok := strings.Cut(v, "=")
		if !ok || id == "" || imp == "" {
			return nil, inputErrorf("-c %q must look like ID=IMPACT, e.g. api=degraded", v)
		}
		if _, dup := out[id]; dup {
			return nil, inputErrorf("component %q is given twice", id)
		}
		out[id] = model.Impact(imp)
	}
	return out, nil
}

func (a *app) author(flag string, p profile) (string, error) {
	for _, c := range []string{flag, p.Author} {
		if c = strings.TrimSpace(c); c != "" {
			return c, nil
		}
	}
	if c := strings.TrimSpace(a.gitUserName()); c != "" {
		return c, nil
	}
	return "", inputErrorf("no author: pass --author, set author in the profile, or set git config user.name")
}
