# statusctl Edit, Retract and Commit Author Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `statusctl` the operations the Mattermost plugin needs: edit an event in place, retract it from the page, record a commit author separate from the name on the page, and accept `2026-11-10 22:00` as a time.

**Architecture:** New pure functions in `ops` (`EditEvent`, `RetractEvent`) return an `ops.Result` that can now carry deletions, a commit-message action and a commit author. Both stores (`Local`, `GitHub`) honour those fields. The CLI exposes `incident|maintenance edit|retract`. This is plan 1 of 2; plan 2 (the `mattermost-plugin-statusctl` repo) is written after this ships as a tagged release.

**Tech Stack:** Go 1.27, cobra, the existing fake GitHub server in `store/fake_github_test.go`.

**Spec:** `docs/superpowers/specs/2026-11-06-mattermost-plugin-statusctl-design.md` (section "1. statusctl: add Edit and Retract", plus "Who can post" for the commit author).

## Global Constraints

- Existing CLI behaviour must not change: when no commit author is set, the commit author name is the latest update's `Author`, exactly as today.
- Edit changes the title and the latest update's message, and for maintenance whose latest status is `scheduled`, `scheduled_start`/`scheduled_end`. It never changes status, impact, timestamps or `Update.Author`.
- Retract deletes `incidents/<id>.json`. Allowed when the event is open, or closed (terminal latest status) no more than 7 days (`7 * 24h`) before `now`. `Force` overrides; only the CLI exposes it.
- Edit and retract record who did it in the commit author name and in the commit message: `<kind> <id>: edited by <name>` / `<kind> <id>: retracted by <name>`.
- `ops.ParseTime` accepts `2006-01-02 15:04` (space) in the site timezone, in addition to RFC 3339 and `2006-01-02T15:04`.
- Every new command takes `--json` and uses the existing exit codes (invalid input = 2).
- CI runs `go vet ./...`, `go test -race ./...`, staticcheck and govulncheck; all must stay clean.

## Review Focus

- An edit that changes nothing (no flags, or values equal to the current ones) must fail with "nothing to change", not write an empty commit. → Task 3
- A whitespace-only `-m " "` on edit means "keep the message", not "a message is required". → Task 3
- Retract after a write conflict: the op re-runs on the new state and the deletion still lands (and fails cleanly if someone else already deleted it). → Task 2, Task 4
- `incident retract` / `incident edit` given a maintenance ID (and vice versa) must refuse with the existing "use the maintenance commands" wording. → Task 3, Task 4
- Editing a maintenance window's end to before its (possibly unchanged) start must be refused. → Task 3

---

### Task 1: Space-separated time form

**Files:**
- Modify: `ops/maintenance.go` (`ParseTime`)
- Modify: `internal/cli/write.go` (help text of `--start`/`--end`)
- Test: `ops/maintenance_test.go`

**Interfaces:**
- Produces: `ops.ParseTime(v string, loc *time.Location) (time.Time, error)` — unchanged signature, now also accepts `"2026-11-10 22:00"`.

- [ ] **Step 1: Write the failing test** `TestParseTimeSpaceForm` in `ops/maintenance_test.go`: with `loc, _ := time.LoadLocation("Europe/Copenhagen")`, `ParseTime("2026-11-10 22:00", loc)` returns `time.Date(2026, 11, 10, 21, 0, 0, 0, time.UTC)`; `ParseTime("2026-11-10 25:00", loc)` returns an `*InputError` whose message mentions `YYYY-MM-DD HH:MM`.
- [ ] **Step 2: Run** `go test ./ops -run TestParseTimeSpaceForm` — expect FAIL.
- [ ] **Step 3: Implement.** Try layout `"2006-01-02 15:04"` with `time.ParseInLocation` after the `T` form. Error text becomes: `time %q must be RFC 3339 (2026-11-02T22:00:00Z) or YYYY-MM-DD HH:MM in the site timezone`. Update the `--start`/`--end` flag help to `"start: YYYY-MM-DD HH:MM in the site timezone, or RFC 3339"` (same for end). Update the doc comment on `ParseTime`.
- [ ] **Step 4: Run** `go test ./...` — expect PASS (fix any existing test asserting the old error text).
- [ ] **Step 5: Commit** `feat(ops): accept "YYYY-MM-DD HH:MM" times`

### Task 2: Result carries deletions, action and commit author; stores honour them

**Files:**
- Modify: `ops/ops.go` (`Result`, `CommitMessage`)
- Modify: `store/github.go` (`commit`)
- Modify: `store/local.go` (`Apply`)
- Modify: `store/fake_github_test.go` (`postTree` handles deletions)
- Test: `ops/ops_test.go` (create if absent; else the file holding `CommitMessage` tests), `store/github_test.go`, `store/local_test.go`

**Interfaces:**
- Produces:
  ```go
  type Result struct {
      Incident     *model.Incident
      URL          string
      Files        map[string][]byte
      Deleted      []string // repository paths to remove
      Action       string   // commit-message suffix; empty means the latest status
      CommitAuthor string   // commit author name; empty means the latest update's author
  }
  func (r *Result) CommitMessage() string    // "<kind> <id>: <Action, else latest status>"
  func (r *Result) CommitAuthorName() string // CommitAuthor, else Incident.Latest().Author
  ```
  A caller (the plugin) may set `CommitAuthor` on a result inside its `store.Op` closure after calling an `ops` function.

- [ ] **Step 1: Write failing tests.**
  - `TestCommitMessageAction` (ops): a result with `Action: "retracted by Bo"` on incident `2026-10-23-x` gives `"incident 2026-10-23-x: retracted by Bo"`; without `Action` it is unchanged (`"incident 2026-10-23-x: investigating"`). `CommitAuthorName()` returns `"Bo"` when `CommitAuthor` is `"Bo"`, else the latest update's author.
  - `TestGitHubCommitAuthorOverride` (store): an op that calls `ops.NewIncident` with `Author: "Fab:IT Support"` then sets `res.CommitAuthor = "Mette"`. Head commit's `author["name"]` is `"Mette"`; the stored incident JSON's update author is `"Fab:IT Support"`.
  - `TestGitHubDeletes` (store): fake starts with `status.yaml` and `incidents/a.json` (a valid incident); an op returning `&ops.Result{Incident: <a>, Deleted: []string{"incidents/a.json"}, Action: "retracted by Bo"}` leaves `headFiles()` without `incidents/a.json` and the head commit message ends in `retracted by Bo`.
  - `TestGitHubDeleteRetriesOnConflict` (store): `pushDirect` a change to an unrelated file between the op's first run and ref update (follow `TestGitHubRetriesOnConflict`'s mechanism); the deletion still lands.
  - `TestLocalDeletes` (store): same as `TestGitHubDeletes` against `Local`; the file is gone from disk.
- [ ] **Step 2: Run** `go test ./ops ./store` — expect FAIL.
- [ ] **Step 3: Implement.**
  - `ops/ops.go`: add the fields and `CommitAuthorName`; `CommitMessage` uses `Action` when set.
  - `store/github.go` `commit`: build tree entries as `[]map[string]any`; written files as today (`path, mode "100644", type "blob", content`); each `Deleted` path as `{"path": p, "mode": "100644", "type": "blob", "sha": nil}` (GitHub's tree API removes a path given `sha: null`). Author name is `res.CommitAuthorName()`.
  - `store/fake_github_test.go` `postTree`: decode entries as `[]map[string]any`; an entry whose `sha` key is present and `nil` deletes the path from the cloned tree.
  - `store/local.go` `Apply`: after writing files, `os.Remove` each `Deleted` path (joined with `filepath.FromSlash`); return the error if it fails.
- [ ] **Step 4: Run** `go test -race ./...` — expect PASS.
- [ ] **Step 5: Commit** `feat(store): support deletions and a separate commit author`

### Task 3: `ops.EditEvent`

**Files:**
- Create: `ops/edit.go`
- Modify: `ops/ops.go` (add `checkKind` helper; reuse it in `UpdateIncident` and `moveMaintenance` for the existing kind-mismatch messages)
- Test: `ops/edit_test.go`

**Interfaces:**
- Consumes: `Result.Action`, `Result.CommitAuthor` (Task 2).
- Produces:
  ```go
  type EditArgs struct {
      Ref        string
      Kind       model.Kind // empty accepts either kind
      Title      string     // empty keeps the title
      Message    string     // blank keeps the latest update's message
      Start, End *time.Time // maintenance still scheduled only; nil keeps
      Author     string     // who edited; recorded in the commit only
  }
  func EditEvent(s *model.State, a EditArgs) (*Result, error)
  ```
  Result: the edited incident, URL and file as from `finish`, `Action: "edited by " + author`, `CommitAuthor: author` (author trimmed).

- [ ] **Step 1: Write failing tests** in `ops/edit_test.go` (use `openIncident` / `scheduleDB` from the helpers):
  - `TestEditMessageAndTitle`: edit `Title: "API latency"`, `Message: "Investigating slow responses."`, `Author: "Bo"`. Latest update's message is the new text; title changed; `len(Updates)`, latest `At`, `Status`, `Impact` and `Author` ("Ada") unchanged; `ID` unchanged; `CommitMessage()` is `"incident 2026-10-23-elevated-api-latency: edited by Bo"`; `CommitAuthorName()` is `"Bo"`.
  - `TestEditOnlyLatestUpdate`: on an incident with two updates, only `Updates[1].Message` changes.
  - `TestEditNothingToChange`: no fields → `InputError` containing `nothing to change`; `Title` equal to current and `Message: "  "` → same error.
  - `TestEditMaintenanceTimes`: on scheduled `scheduleDB`, `End` moved to 23:30 UTC → `ScheduledEnd` updated, start unchanged; `End` at 20:00 UTC (before unchanged 21:00 start) → error containing `end must be after the start`.
  - `TestEditTimesRefused`: `Start` on an incident → error containing `only maintenance`; `Start` on maintenance after `StartMaintenance` → error containing `before it starts`.
  - `TestEditKindMismatch`: `Kind: model.KindIncident` on a maintenance ID → error containing `use the maintenance commands`; and the reverse → `use the incident commands`.
  - `TestEditValidates`: a two-line title → title error; missing `Author` → `an author is required`.
- [ ] **Step 2: Run** `go test ./ops -run TestEdit` — expect FAIL.
- [ ] **Step 3: Implement** `EditEvent` in `ops/edit.go`: `Find`, `checkKind`, `checkAuthor`; clone; apply trimmed title (via `checkTitle`) and non-blank message (via `checkMessage`) to the clone; times only when kind is maintenance and latest status is `scheduled`, `stamp`ed, with end-after-start checked on the merged values; if the clone equals the current incident (compare marshalled bytes), return `nothing to change: pass a new title, message, start or end`; `finish`, then set `Action` and `CommitAuthor`. `checkKind(in *model.Incident, want model.Kind) error` returns nil for empty `want`, otherwise the existing messages `"%s is a maintenance window; use the maintenance commands"` / `"%s is an incident; use the incident commands"`.
- [ ] **Step 4: Run** `go test ./ops` — expect PASS.
- [ ] **Step 5: Commit** `feat(ops): edit an event's title, message and maintenance times`

### Task 4: `ops.RetractEvent`

**Files:**
- Create: `ops/retract.go`
- Test: `ops/retract_test.go`

**Interfaces:**
- Consumes: `Result.Deleted`, `Result.Action`, `Result.CommitAuthor` (Task 2); `checkKind` (Task 3).
- Produces:
  ```go
  const RetractWindow = 7 * 24 * time.Hour
  type RetractArgs struct {
      Ref    string
      Kind   model.Kind // empty accepts either kind
      Author string     // who retracted; recorded in the commit only
      Force  bool       // allow events closed longer than RetractWindow ago
  }
  func RetractEvent(s *model.State, a RetractArgs, now time.Time) (*Result, error)
  ```
  Result: `Incident` is the event as it was (for callers to describe it), `URL` is the site root (`strings.TrimRight(Site.URL, "/") + "/"`), `Files` nil, `Deleted: []string{in.Path()}`, `Action: "retracted by " + author`, `CommitAuthor: author`.

- [ ] **Step 1: Write failing tests** in `ops/retract_test.go`:
  - `TestRetractOpen`: open incident, `Author: "Bo"` → `Deleted == []string{"incidents/2026-10-23-elevated-api-latency.json"}`, `URL == "https://status.example.com/"`, `CommitMessage()` ends `retracted by Bo`.
  - `TestRetractRecentlyClosed`: resolved at `now`, retract at `now + 7*24h` → ok.
  - `TestRetractOldRefused`: resolved at `now`, retract at `now + 7*24h + time.Second` → error containing `more than 7 days ago`; same with `Force: true` → ok.
  - `TestRetractOpenNeverExpires`: still-open incident retracted 30 days later without `Force` → ok.
  - `TestRetractKindMismatch` and missing author, as in Task 3.
- [ ] **Step 2: Run** `go test ./ops -run TestRetract` — expect FAIL.
- [ ] **Step 3: Implement.** Closed means `Latest().Status.Terminal()`; age is `now.Sub(LastUpdated())`. Refusal text: `"%s closed more than 7 days ago; retracting it would rewrite history (pass --force to do it anyway)"`.
- [ ] **Step 4: Run** `go test ./ops ./store` — expect PASS.
- [ ] **Step 5: Commit** `feat(ops): retract an event from the page`

### Task 5: CLI `edit` and `retract`, docs

**Files:**
- Modify: `internal/cli/write.go` (`mutate` output; new `editCmd(kind)`, `retractCmd(kind)` registered under `incident` and `maintenance`)
- Modify: `README.md` ("Post updates" section)
- Test: `internal/cli/write_test.go`

**Interfaces:**
- Consumes: `ops.EditEvent`, `ops.EditArgs`, `ops.RetractEvent`, `ops.RetractArgs`, `Result.Action`.
- Produces (CLI):
  - `statusctl incident edit ID [--title T] [-m TEXT | -F FILE] [--author NAME]`
  - `statusctl maintenance edit ID [--title T] [-m | -F] [--start T] [--end T] [--author NAME]`
  - `statusctl incident retract ID [--force] [--author NAME]`, `statusctl maintenance retract ID [--force] [--author NAME]`
  - `--json` output gains `"action"` when `Result.Action` is set; human output prints `<kind> <id>: <action>` then the URL instead of the status label.

- [ ] **Step 1: Write failing tests** in `internal/cli/write_test.go`, using `newRepo`/`run`/`wantCode`/`loadRepo`:
  - `TestIncidentEditAndRetract`: open an incident; `incident edit 2026-10-23-api -m "Better words." --author Bo --json` exits 0, JSON `action` is `"edited by Bo"`, the file's latest message is `"Better words."` and its author is still `"Git User"`; `incident edit 2026-10-23-api` with no flags exits 2 mentioning `nothing to change` (and must not open the editor); `incident retract 2026-10-23-api` exits 0, stderr mentions `retracted by Git User`, and `incidents/2026-10-23-api-slow.json` no longer exists.
  - `TestMaintenanceEditTimes`: schedule with `--start "2026-11-02 22:00" --end "2026-11-03 00:00"`; `maintenance edit <id> --end "2026-11-03 01:00"` exits 0 and `ScheduledEnd` is `2026-11-03T00:00:00Z`; `incident retract <maintenance id>` exits 2 mentioning `maintenance commands`.
  - `TestRetractForce`: with `newTestApp` and `now` moved 8 days past resolution, `incident retract` exits 2 mentioning `--force`; with `--force` exits 0.
- [ ] **Step 2: Run** `go test ./internal/cli -run 'Edit|Retract'` — expect FAIL.
- [ ] **Step 3: Implement.** Edit reads the message with `a.message(msg, false)` (never opens the editor). `--start`/`--end` parse with `ops.ParseTime` inside the op, as `maintenanceNewCmd` does, and are only registered on `maintenance edit`. `--author` help: `"your name, recorded in the commit (default: profile author, then git user.name)"`. Pass `Kind` matching the parent command. README: add to the "Post updates" block:
  ```sh
  statusctl incident edit 2026-10-23-elev -m "Corrected wording."
  statusctl incident retract 2026-10-23-elev          # false alarm: removes it from the page
  statusctl maintenance edit 2026-11-02-db --end "2026-11-03 01:00"
  ```
  and one sentence: retract only works within 7 days of an event closing unless `--force` is given; git history keeps the record. Change the maintenance example's times to the `"2026-11-02 22:00"` form.
- [ ] **Step 4: Run** `go vet ./... && go test -race ./...` — expect PASS.
- [ ] **Step 5: Commit** `feat(cli): incident and maintenance edit and retract`

### Task 6: Release v0.2.0

Outward-facing: **ask the user before merging, tagging or pushing.**

- [ ] **Step 1:** Run the CI checks locally: `go vet ./... && go test -race ./... && go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./... && go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...` — expect no output beyond test `ok` lines.
- [ ] **Step 2:** With the user's go-ahead: merge the branch into `main`, tag `v0.2.0`, push `main` and the tag. The release workflow builds the binaries.
- [ ] **Step 3:** Confirm the release exists with `gh release view v0.2.0` and that `go list -m github.com/Fab-IT-ApS/statusctl@v0.2.0` resolves (the plugin depends on it).
