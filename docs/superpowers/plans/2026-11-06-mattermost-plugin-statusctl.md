# mattermost-plugin-statusctl Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A server-only Mattermost plugin that lets anyone in an org's status channel post and maintain that org's status page through `/status`, forms and buttons, using `statusctl` v0.2.0's Go packages.

**Architecture:** One Go server plugin. A config layer maps each configured status channel to an org (repo, branch, token, public label). Pure builders turn statusctl state into dialogs and channel cards, and turn dialog submissions into `ops` calls. A thin plugin layer wires the slash command, the HTTP endpoints for dialog submissions and button clicks, and posting as a bot. All writes go through `store.Store.Apply`, so they retry on conflict the way the CLI's do.

**Tech Stack:** Go 1.27; `github.com/Fab-IT-ApS/statusctl v0.2.0` (`model`, `ops`, `store`, `derive`); `github.com/mattermost/mattermost/server/public v0.1.16` (`plugin`, `model`, `pluginapi`, `plugin/plugintest`). v0.1.16 is the SDK released with Mattermost 10.11 (July 2025). Pinning it means the code can't call any API newer than the server.

**Spec:** `docs/superpowers/specs/2026-11-06-mattermost-plugin-statusctl-design.md` (statusctl repo).

**Where:** A new repo `Fab-IT-ApS/mattermost-plugin-statusctl`, checked out at `/home/jrisch/Development/Claude/mattermost-plugin-statusctl`. Task 1 creates it locally. Creating the GitHub repo and pushing are in Task 10 and need the user's go-ahead.

## Global Constraints

- Plugin ID `dk.fabit.statusctl`; `min_server_version` `10.11.0`; server-only (no webapp); MIT licence.
- Must work on self-hosted Team Edition 10.11. Use no API, dialog element type, or setting type absent from SDK v0.1.16. In particular, there is no `datetime` element.
- Dialog field label (`display_name`) is limited to 24 characters, help text to 150, `text` elements to 150, and `textarea` elements to 3000.
- `/status` only acts in a configured status channel. Anywhere else, it replies ephemerally and names that team's status channel if one is configured.
- Anyone who can see the status channel can post: they must be a member of the channel. There is no approval step, and changes go live immediately.
- `Update.Author` is always the org's **public label**. The person is recorded as `Result.CommitAuthor` and named on the card. Their display name is the full name, falling back to the username.
- Every change posts a card in the channel showing who made it, the title, the status, the affected components in plain words, the message as published, and the page link (`site.url`).
- Plain words only in the UI. Impacts are *Working normally / Slow or degraded / Partly down / Down* (= `operational / degraded / partial_outage / major_outage`). Never show impact IDs, event IDs (except in typed-command output), or ISO timestamps beyond the maintenance example `2026-11-10 22:00`.
- Maintenance never starts or completes by itself, and the plugin runs no timers. The scheduled card says: "This won't start by itself. Press Start when the work begins."
- Tokens never appear in posts, ephemeral messages, logs, or errors shown to users.
- After any submit, the user must be able to tell whether the page changed: a card on success, or a plain error saying "The status page was not changed" on failure.

## Review Focus

- A button pressed on an old card when the event has since moved on (e.g. Resolve on an already-resolved incident, Start on completed maintenance): the user gets a plain explanation and nothing is written. → Task 7
- A component name longer than 24 characters, or a `status.yaml` with many components: the dialog is still valid (label truncated, full name in help text) and opens. → Task 5
- Someone who is not a member of the status channel calling the action or dialog endpoints directly, e.g. with a crafted request: refused, nothing written. → Task 7
- A typed command with quoted titles containing spaces or Danish letters (`/status incident new "Mail på vej ned" -c mail=down`) parses correctly. → Task 8
- A GitHub failure (bad token, network down, conflict exhausted) shows the user a plain message, never the token or a raw API body, and says the page wasn't changed. → Task 4

---

### Task 1: Repository scaffold, manifest, bundle build

**Files:**
- Create: `go.mod`, `plugin.json`, `server/main.go`, `server/plugin.go`, `server/manifest_test.go`, `Makefile`, `.gitignore`, `LICENSE`, `README.md` (stub: one paragraph plus "see docs/"), `.github/workflows/ci.yml`
- Copy: this plan into `docs/plans/` and the spec into `docs/design.md` in the new repo, so the repo is self-describing.

**Interfaces:**
- Produces: `type Plugin struct { plugin.MattermostPlugin; client *pluginapi.Client; botID string; ... }`. The fields later tasks add are named in those tasks. `OnActivate() error` creates the client, ensures the bot (`username: "status"`, display name `Status`), and registers the command (Task 8 supplies `commandSpec()`, a stub until then).

- [ ] **Step 1:** `git init` at `/home/jrisch/Development/Claude/mattermost-plugin-statusctl`. Module `github.com/Fab-IT-ApS/mattermost-plugin-statusctl`, `go 1.27.3`. `go get github.com/Fab-IT-ApS/statusctl@v0.2.0 github.com/mattermost/mattermost/server/public@v0.1.16`.
- [ ] **Step 2: Write the failing test** `TestManifest` in `server/manifest_test.go`: read `../plugin.json` and decode it with `model.ManifestFromJSON`. Assert `ID == "dk.fabit.statusctl"`, `MinServerVersion == "10.11.0"`, `HasServer()`, `!HasWebapp()`, and that `SettingsSchema.Settings` has exactly one setting with key `Organisations`, type `longtext`.
- [ ] **Step 3: Run** `go test ./server/...` and expect FAIL (no plugin.json).
- [ ] **Step 4:** Write `plugin.json`. It needs name `Status page`, a description, and `server.executables` for `linux-amd64` and `linux-arm64` at `server/dist/plugin-linux-{arch}`. Its settings schema holds one `longtext` setting, `Organisations`. Help text: "One entry per org, as JSON. See the admin guide." Placeholder: a one-entry example with `"token": "github_pat_..."`. Then write `server/main.go` (`plugin.ClientMain(&Plugin{})`) and `server/plugin.go`.
- [ ] **Step 5:** `Makefile` target `dist`: `CGO_ENABLED=0 go build -trimpath -o server/dist/plugin-linux-$arch ./server` for amd64 and arm64. Then tar `plugin.json` and `server/dist/` under a top-level folder `dk.fabit.statusctl/` into `dist/dk.fabit.statusctl-$(VERSION).tar.gz`, with `VERSION` taken from `plugin.json`'s `version`, starting at `0.1.0`. Add a `test` target. `.github/workflows/ci.yml` mirrors statusctl's: vet, `go test -race ./...`, staticcheck v0.8.2, govulncheck v1.8.0, and `make dist`. Pin actions by SHA, the same as statusctl.
- [ ] **Step 6: Run** `go test ./... && make dist && tar tzf dist/dk.fabit.statusctl-0.1.0.tar.gz`. Expect PASS, with the listing including `dk.fabit.statusctl/plugin.json` and both executables.
- [ ] **Step 7: Commit** `chore: scaffold server plugin`

### Task 2: Org configuration

**Files:**
- Create: `server/orgs.go`, `server/orgs_test.go`
- Modify: `server/plugin.go` (`OnConfigurationChange`)

**Interfaces:**
- Produces:
  ```go
  type Org struct {
      Team        string `json:"team"`         // team name (URL name)
      Channel     string `json:"channel"`      // channel name (URL name)
      Repo        string `json:"repo"`         // owner/name
      Branch      string `json:"branch"`       // default "main"
      Token       string `json:"token"`
      PublicLabel string `json:"public_label"` // shown on the page as the author
      TeamID, ChannelID string `json:"-"`
  }
  func ParseOrgs(raw string) ([]Org, error)        // validates; errors never include token values
  type Registry struct{ /* by channel ID, by team ID */ }
  func NewRegistry(orgs []Org) *Registry            // orgs with IDs resolved
  func (r *Registry) ForChannel(channelID string) (Org, bool)
  func (r *Registry) ForTeam(teamID string) (Org, bool)
  ```
  `Plugin` gains `registry atomic.Pointer[Registry]`.

- [ ] **Step 1: Write failing tests:**
  - `TestParseOrgs`: a two-entry JSON array parses. A missing branch becomes `"main"`.
  - `TestParseOrgsRejects`: empty `team`, `channel`, `repo`, `token` or `public_label` → error naming the entry (`entry 2: public_label is required`). A repo without `/` → error. Two entries for the same team or channel → error. Invalid JSON → error. An error built from an entry with token `secret-token-123` never contains `secret-token-123`.
  - `TestParseOrgsEmpty`: `""` and `"[]"` → no orgs, no error.
  - `TestRegistry`: `ForChannel` and `ForTeam` find the right org; unknown IDs → false.
- [ ] **Step 2: Run** `go test ./server -run 'Orgs|Registry'` and expect FAIL.
- [ ] **Step 3: Implement.** `OnConfigurationChange` loads `struct{ Organisations string }`, parses it, and resolves team and channel names via `API.GetTeamByName` and `API.GetChannelByName(teamID, name, false)`. An entry that can't be resolved is logged with `LogError` (team and channel names only) and left out. The rest still load, and that channel then gets the "not set up" reply. A parse error is returned, so Mattermost logs it, and the previous registry stays in place.
- [ ] **Step 4: Run** `go test ./server` and expect PASS.
- [ ] **Step 5: Commit** `feat: per-org configuration`

### Task 3: Plain words

**Files:**
- Create: `server/words.go`, `server/words_test.go`

**Interfaces:**
- Produces:
  ```go
  var impactChoices []model.Impact // operational, degraded, partial_outage, major_outage, in that order
  func impactWord(i model.Impact) string // "Working normally", "Slow or degraded", "Partly down", "Down", "Under maintenance"
  func statusWord(s model.Status) string // model.Status.Label()
  func impactOptions() []*mmodel.PostActionOptions // Text = impactWord, Value = impact ID
  func fieldLabel(name string) string // ≤24 runes; longer names cut to 23 runes + "…"
  ```
  (`mmodel` = Mattermost `model`; `model` = statusctl `model`, in every file.)

- [ ] **Step 1: Write failing tests:** `TestImpactWords` covers all five impacts. `TestFieldLabel`: `"Public API"` is unchanged; a 30-rune name with `æøå` returns 24 runes ending in `…`.
- [ ] **Step 2: Run**, FAIL. **Step 3: Implement.** **Step 4: Run**, PASS.
- [ ] **Step 5: Commit** `feat: plain-word labels`

### Task 4: Applying a change

**Files:**
- Create: `server/apply.go`, `server/apply_test.go`

**Interfaces:**
- Consumes: `Org` (Task 2).
- Produces:
  ```go
  // Plugin gains: newStore func(Org) (store.Store, error) — defaults to store.NewGitHub(org.Repo, org.Branch, org.Token); now func() time.Time
  type change func(s *model.State, label string, now time.Time) (*ops.Result, error)
  func (p *Plugin) apply(ctx context.Context, org Org, person string, c change) (*ops.Result, error)
  func (p *Plugin) load(ctx context.Context, org Org) (*model.State, error)
  func userMessage(err error) string
  ```
  `apply` runs `c` inside `store.Apply`, passing `org.PublicLabel` as `label`, and sets `res.CommitAuthor = person` on success. `userMessage` returns:
  - `*ops.InputError` or `*model.ValidationError`: its text.
  - `store.ErrConflict`: "Someone else changed the status page at the same moment. Nothing was changed — please try again."
  - `store.ErrAuth` or `store.ErrNetwork`: "The status page could not be reached, so nothing was changed. Please tell an admin." The full error is logged server-side with the token never included. statusctl's errors already exclude it.
  - Anything else: the same "tell an admin" text.

- [ ] **Step 1: Write failing tests** with `p.newStore` returning a `store.Local` on a temp dir that holds a `status.yaml` (components `api` "Public API" and `web` "Customer portal"):
  - `TestApplyUsesLabelAndPerson`: apply an `ops.NewIncident` change as person `"Mette Hansen"` with label `"Fab:IT Support"`. The stored update author is `"Fab:IT Support"`, and `res.CommitAuthorName()` is `"Mette Hansen"`.
  - `TestUserMessage`: each error class above maps to its text. An error wrapping `store.ErrAuth` with the text `token=abc` doesn't contain `abc`.
- [ ] **Step 2: Run**, FAIL. **Step 3: Implement.** **Step 4: Run**, PASS.
- [ ] **Step 5: Commit** `feat: apply changes as the public label, recording the person`

### Task 5: Dialogs

**Files:**
- Create: `server/dialogs.go`, `server/dialogs_test.go`

**Interfaces:**
- Consumes: `fieldLabel`, `impactOptions`, `impactWord` (Task 3).
- Produces: one builder and one parser per form. A builder returns `mmodel.Dialog` with `CallbackId` set to the form name and `State` set to the event ID, or empty for new events. The caller fills the URL. A parser takes the submission plus the current state and returns ops arguments or field errors (`map[string]string`, keyed by element name).
  ```go
  const (formReport = "report"; formUpdate = "update"; formResolve = "resolve"; formEdit = "edit"
         formMaintenance = "maintenance"; formCancel = "cancel"; formRetract = "retract")
  func reportDialog(cfg *model.Config) mmodel.Dialog
  func parseReport(cfg *model.Config, sub map[string]any) (ops.NewIncidentArgs, map[string]string)
  func updateDialog(cfg *model.Config, in *model.Incident) mmodel.Dialog
  func parseUpdate(cfg *model.Config, in *model.Incident, sub map[string]any) (ops.UpdateArgs, map[string]string)
  func resolveDialog(in *model.Incident) mmodel.Dialog
  func editDialog(cfg *model.Config, in *model.Incident) mmodel.Dialog
  func parseEdit(cfg *model.Config, in *model.Incident, sub map[string]any) (ops.EditArgs, map[string]string)
  func maintenanceDialog(cfg *model.Config) mmodel.Dialog
  func parseMaintenance(cfg *model.Config, sub map[string]any) (ops.NewMaintenanceArgs, map[string]string)
  func cancelDialog(in *model.Incident) mmodel.Dialog
  func retractDialog(in *model.Incident) mmodel.Dialog
  ```
  Parsers leave `Author` empty and fill `Ref` with the event ID. The plugin layer sets the author from `label`.

  Form contents, from the spec:
  - **report** ("Report a problem", submit "Post to status page"): `title` (text, "What's wrong?", help "Shown as the headline on the status page."). Then one select per active component, named `c:<id>`, with label `fieldLabel(name)`, help set to the full name only when it was truncated, options from `impactOptions()`, and default `operational`. Then `message` (textarea, optional, "Message to customers", help `Leave blank for "We are investigating."`). The parser requires at least one component that isn't operational ("Choose at least one thing that isn't working normally."). It sends only non-operational components as `Impacts`, and uses `"We are investigating."` for a blank message.
  - **update** ("Post an update"): `status` (select, Investigating/Identified/Monitoring, defaults to the current one). One `c:<id>` select per component of the incident, defaulting to its current impact. `message` is required.
  - **resolve** ("Resolve"): `message`, optional, default text "This incident has been resolved." (my call: an incident needs a message, so the plugin supplies this one).
  - **edit** ("Edit"): `title`, defaulting to the current title. `message`, defaulting to the latest message. For maintenance whose latest status is `scheduled`, also `start` and `end` (text, default from `ScheduledStart`/`ScheduledEnd` formatted `2006-01-02 15:04` in the site timezone). `parseEdit` sends only the fields that changed. If nothing changed, it returns `{"message": "Nothing was changed."}`.
  - **maintenance** ("Schedule maintenance"): `title`. `c:<id>` as a `bool` per active component ("Affected"). `start` and `end` (text, placeholder `2026-11-10 22:00`, help "Time in <timezone name>"). `message` is optional. Bad times give the error `"Write the time like 2026-11-10 22:00"` on that field, using `ops.ParseTime`. End not after start → error on `end`. No component ticked → error on the first component field.
  - **cancel** ("Cancel maintenance"): `message` is required (ops requires it).
  - **retract** ("Remove from status page", submit "Remove"): no fields. Introduction text: `"<title>" will disappear from the status page. Customers won't see it any more. The record stays in the status repository's history.`

- [ ] **Step 1: Write failing tests.** Use a config with a component named `"Very long component name for testing"` (36 runes):
  - `TestReportDialogValid`: `(&reportDialog(cfg)).IsValid() == nil`. Each component field's `DisplayName` is 24 runes or fewer, and the long one has help text with the full name.
  - `TestParseReport`: `{title: "Portal slow", "c:web": "degraded", "c:api": "operational", message: ""}` gives `Impacts == {web: degraded}` and message `"We are investigating."`. Every component operational → an error on the first component field. A blank title → an error on `title`.
  - `TestUpdateDialogDefaults`: the defaults match the incident's current status and impacts, and the dialog is valid.
  - `TestParseEditOnlyChanges`: an unchanged title and a changed message give `EditArgs{Title: "", Message: "new"}`. Nothing changed → an error.
  - `TestEditDialogTimesOnlyWhenScheduled`: `start`/`end` appear only for scheduled maintenance, with defaults in Europe/Copenhagen time.
  - `TestParseMaintenance`: `"2026-11-10 22:00"`/`"2026-11-10 23:00"` parse to 21:00Z/22:00Z. `"tomorrow"` → an error on `start`. End before start → an error on `end`. No components → an error.
  - `TestAllDialogsValid`: every builder's output passes `IsValid()` for a config with 12 components.
- [ ] **Step 2: Run** `go test ./server -run Dialog\|Parse` and expect FAIL. **Step 3: Implement.** **Step 4: Run**, PASS.
- [ ] **Step 5: Commit** `feat: forms for every change`

### Task 6: Cards

**Files:**
- Create: `server/cards.go`, `server/cards_test.go`

**Interfaces:**
- Consumes: `impactWord`, `statusWord` (Task 3).
- Produces:
  ```go
  type action string // "update","resolve","edit","retract","start","complete","cancel"
  func cardActions(in *model.Incident, now time.Time) []action
  func eventCard(cfg *model.Config, in *model.Incident, person, verb, actionURL string, now time.Time) *mmodel.SlackAttachment
  func retractNote(in *model.Incident, person string) string
  func statusSummary(s *model.State, actionURL string, now time.Time) []*mmodel.SlackAttachment // for /status show
  ```
  The `verb` is one of "posted an incident", "updated", "resolved", "edited", "scheduled maintenance", "started", "completed" or "cancelled". The pretext reads `@<person> <verb> "<title>" on <site.url>`. The card's colour follows the worst current impact: green (operational), amber (degraded or under maintenance), red (partial or major outage).

  The attachment's fields:
  - **Status**: `statusWord`.
  - **Affected**: one line per component, `<name>: <impactWord>`.
  - **Message**: the latest message.
  - **Page**: `<site.url>` (the incident's own page for open events).
  - **When**, maintenance only: start and end, formatted `Mon 2 Jan 15:04` in the site timezone.

  A scheduled maintenance card adds the footer "This won't start by itself. Press Start when the work begins."

  Each action becomes a button whose `Integration.URL` is `actionURL + "/" + action` and whose `Context` is `{"event": in.ID}`.

  `cardActions`, by latest status:
  - Open incident: update, resolve, edit, retract.
  - Resolved incident: edit, plus retract while `now - LastUpdated <= ops.RetractWindow`.
  - Scheduled: start, edit, cancel, retract.
  - In progress: complete, edit, retract.
  - Completed or cancelled: edit, plus retract within the window.

  `retractNote` reads: `@<person> removed "<title>" from the status page. It was posted <Mon 2 Jan 15:04>. The record stays in the status repository's history.`

- [ ] **Step 1: Write failing tests:**
  - `TestCardActions`: covers each row above, including a resolved incident at 7 days (retract present) and at 7 days plus 1 second (absent).
  - `TestEventCardIncident`: the pretext names the person and the title. The fields show `"Customer portal: Slow or degraded"` and the message. The colour is amber. There are 4 buttons, with URL `…/action/update` and context event ID. No impact IDs appear anywhere in the attachment text.
  - `TestEventCardScheduled`: the footer contains "won't start by itself", and the When field is in site time.
  - `TestStatusSummary`: the first attachment lists every active component with its word. One attachment per active or upcoming event follows, with its buttons.
- [ ] **Step 2: Run**, FAIL. **Step 3: Implement.** **Step 4: Run**, PASS.
- [ ] **Step 5: Commit** `feat: channel cards`

### Task 7: HTTP endpoints for buttons and dialog submissions

**Files:**
- Create: `server/http.go`, `server/http_test.go`

**Interfaces:**
- Consumes: Tasks 2–6.
- Produces:
  - `ServeHTTP` routes `POST /action/{action}` (body: `mmodel.PostActionIntegrationRequest`) and `POST /dialog/{form}` (body: `mmodel.SubmitDialogRequest`).
  - `func (p *Plugin) actionURL() string` returns `"/plugins/dk.fabit.statusctl/action"`. `dialogURL(form)` follows the same pattern.
  - `func (p *Plugin) personName(userID string) string` returns the full name, falling back to the username.
  - `func (p *Plugin) postCard(org Org, att *mmodel.SlackAttachment)` posts as the bot in `org.ChannelID`.

  For both routes, `Mattermost-User-Id` must be present, the request's channel must be a configured status channel, and `API.GetChannelMember(channelID, userID)` must succeed. Otherwise the response is 403 with nothing written. Use the header, never the user ID in the body.

  **Buttons:**
  1. Load the state and find the event from the context.
  2. If the action is not in `cardActions(event, now)`, return an `EphemeralText` explaining the current state, for example: `"<title>" is already resolved, so there's nothing to resolve. Nothing was changed.` Then stop.
  3. Otherwise, the form actions (update, resolve, edit, cancel, retract) open the dialog with the request's `TriggerId`.
  4. Start and complete apply directly, with ops' default messages, and post a card.

  **Dialogs:**
  - A cancelled submission returns 200 with no body.
  - Parse the submission. Field errors come back as a `SubmitDialogResponse{Errors}`, so the user's input stays in the form.
  - Otherwise apply the change, with `Author` set to `label` in a `change`.
  - On failure, return `SubmitDialogResponse{Error: userMessage(err)}`. On success, post the card, or `retractNote` for a retract, and return 200 with no body.

- [ ] **Step 1: Write failing tests** with `plugintest.API`, mocking `GetChannelMember`, `GetUser`, `CreatePost`, `OpenInteractiveDialog` and `LogError`. Use `p.newStore` returning `store.Local` on a temp repo:
  - `TestActionRequiresMember`: `GetChannelMember` returns an error → 403. No `CreatePost` or `OpenInteractiveDialog` call (`api.AssertNotCalled`).
  - `TestActionRequiresHeader`: no `Mattermost-User-Id` → 401.
  - `TestActionOutsideStatusChannel`: an unconfigured channel → 403.
  - `TestStaleResolve`: an incident already resolved, then the resolve action → `EphemeralText` contains "already resolved". The repo is unchanged.
  - `TestStartAppliesAndPosts`: scheduled maintenance, then start → the repo shows in progress, the author on the page is the label, and one `CreatePost` from the bot in the channel with a card naming the person.
  - `TestReportSubmission`: a valid report → the incident file exists and `CreatePost` is called once. An invalid submission → `Errors` has `title` and nothing is written.
  - `TestRetractSubmission`: the file is removed, and the posted message is `retractNote`.
  - `TestSubmissionStoreFailure`: `newStore` returns a store whose `Apply` returns `store.ErrAuth` → `Error` is the "tell an admin" text, and there is no `CreatePost`.
- [ ] **Step 2: Run**, FAIL. **Step 3: Implement.** **Step 4: Run** `go test -race ./...`, PASS.
- [ ] **Step 5: Commit** `feat: buttons and form submissions`

### Task 8: The `/status` command

**Files:**
- Create: `server/command.go`, `server/args.go`, `server/command_test.go`, `server/args_test.go`
- Modify: `server/plugin.go` (register the real `commandSpec()`)

**Interfaces:**
- Consumes: Tasks 2–7.
- Produces:
  - `func commandSpec() *mmodel.Command`: trigger `status`, `AutoComplete: true`, and `AutocompleteData` with subcommands `incident`, `maintenance`, `show`, `list` and `help`. Their help texts are "Report a problem (typed)", "Schedule maintenance (typed)", "What the page says right now", "Open incidents and maintenance" and "How to use this".
  - `func (p *Plugin) ExecuteCommand(c *plugin.Context, args *mmodel.CommandArgs) (*mmodel.CommandResponse, *mmodel.AppError)`.
  - `func splitArgs(s string) ([]string, error)`: shell-like splitting with `"…"` and `'…'` quoting. An unclosed quote is an error.

  **Behaviour:**
  - Outside a status channel: an ephemeral reply, `/status only works in ~<channel>.`, or `/status isn't set up for this team. Ask an admin.` when the team has no status channel.
  - Bare `/status`: an ephemeral post with one attachment and three buttons: **Report a problem** (primary style), **Schedule maintenance** and **Current status**. They go to the action endpoint as actions `menu-report`, `menu-maintenance` and `menu-show`. Task 7's router gets these three; they open the report or maintenance dialog, or reply with `statusSummary`. They need channel membership like every other action, but no event.
  - `show` and `list`: the `statusSummary` attachments, ephemeral.
  - `help`: a short ephemeral text, ending with a link to `docs/user-guide.md` on GitHub (`https://github.com/Fab-IT-ApS/mattermost-plugin-statusctl/blob/main/docs/user-guide.md`).
  - Typed commands mirror the CLI, parsed with `pflag`:
    - `incident new TITLE -c ID=IMPACT… [-m TEXT]`
    - `incident update ID [--status S] [-c ID=IMPACT…] -m TEXT`
    - `incident resolve ID [-m TEXT]`
    - `incident edit ID [--title T] [-m TEXT]`
    - `incident retract ID`
    - `maintenance new TITLE -c ID… --start T --end T [-m TEXT]`
    - `maintenance start|complete ID [-m TEXT]`
    - `maintenance cancel ID -m TEXT`
    - `maintenance edit ID [--title] [-m] [--start] [--end]`
    - `maintenance retract ID`

    Impacts accept both the IDs and the words `down` and `partly-down`. IDs accept unique prefixes, as `ops.Find` already does. On success the card is posted in the channel, and the ephemeral reply is the event ID. On error the reply is an ephemeral `userMessage`. Typed retract obeys the 7-day window, with no `--force`.

- [ ] **Step 1: Write failing tests:**
  - `TestSplitArgs`: `incident new "Mail på vej ned" -c mail=down` gives `[incident new, Mail på vej ned, -c, mail=down]`. `'a b'` gives `[a b]`. An unclosed quote → an error.
  - `TestCommandOutsideChannel`: the reply names `~status` for a team with an org, and "isn't set up" for a team without one.
  - `TestBareCommandMenu`: the ephemeral post has 3 buttons and the first is primary.
  - `TestTypedIncidentNew`: after `/status incident new "API slow" -c api=degraded -m "Looking."` the repo has the incident, the page author is the label, and a card is posted.
  - `TestTypedErrors`: an unknown component, a bad flag and a missing title each give an ephemeral reply and write nothing.
  - `TestCommandSpecValid`: `commandSpec().AutocompleteData.IsValid() == nil`.
- [ ] **Step 2: Run**, FAIL. **Step 3: Implement.** **Step 4: Run** `go test -race ./...`, PASS.
- [ ] **Step 5: Commit** `feat: /status command`

### Task 9: Documentation

**Files:**
- Create: `docs/user-guide.md`, `docs/admin-guide.md`, `docs/images/.gitkeep`
- Modify: `README.md`

**Interfaces:** none. The help link in Task 8 points at `docs/user-guide.md`.

- [ ] **Step 1:** **User guide**, plain language, for people in sales and support, with no CLI terms. Sections:
  1. What this is.
  2. Report a problem.
  3. Post an update.
  4. Resolve.
  5. Schedule maintenance, saying plainly that it won't start by itself.
  6. Start and complete maintenance.
  7. Fix a mistake (Edit and Remove), including the 7-day limit on removing.
  8. Check what the page says.
  9. Typed commands, for those who like them.

  Each section has an image slot, e.g. `![The report form](images/report-form.png)`. Screenshots are captured in Task 10 Step 4.
- [ ] **Step 2:** **Admin guide**, covering:
  - Enabling plugin uploads, then uploading the bundle (System Console → Plugins → Plugin Management).
  - Creating the fine-grained token: one repo, Contents read and write.
  - The `Organisations` JSON, with a full example and every field explained. `team` and `channel` are the URL names. `public_label` is what customers see.
  - Only system admins can see the token in the System Console.
  - Adding a second org.
  - Upgrading the plugin.
  - Moving to Mattermost v11 later: the date picker is a planned follow-up.
  - Troubleshooting: "not set up", "tell an admin" errors, where the server logs are.
- [ ] **Step 3:** **README**: what the plugin does, a link to both guides, a build line (`make dist`), and the licence.
- [ ] **Step 4:** Check every relative link in `docs/*.md` and `README.md` resolves. Use `grep -o '](\([^)]*\))'` and confirm each target exists, apart from the image slots, which are listed in Task 10.
- [ ] **Step 5: Commit** `docs: user and admin guides`

### Task 10: Release and first install

Outward-facing. **Ask the user before creating the GitHub repo, pushing, tagging or installing on their server.**

- [ ] **Step 1:** Run the CI checks locally: `go vet ./... && go test -race ./... && go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./... && go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./... && make dist`. Expect no failures.
- [ ] **Step 2:** Add `.github/workflows/release.yml`: on a `v*` tag, run the tests and `make dist`, write `SHA256SUMS`, and run `gh release create` with the bundle. Mirror statusctl's release workflow.
- [ ] **Step 3:** With the user's go-ahead: create `Fab-IT-ApS/mattermost-plugin-statusctl` (public), push `main`, tag `v0.1.0`, and push the tag.
- [ ] **Step 4:** With the user: install the bundle on the Mattermost server and configure a test org pointing at a scratch status repo. Walk through every user-guide section once, capture the screenshots into `docs/images/`, and commit them (`docs: screenshots`).
