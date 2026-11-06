# Mattermost plugin for statusctl — design

Date: 2026-11-06
Status: approved

## Intent

Fab:IT runs status pages for several organisations with `statusctl` (this repo):
each org's status lives as JSON in its own GitHub repo, and Netlify renders it
into a static page (e.g. status.fab-it.dk). Today, posting an update means
running the CLI, which only technical staff will do.

The goal is that **anyone in the company** — including non-technical people in
support or sales — can post and maintain status updates from inside Mattermost,
in a shared status channel, without installing or knowing `statusctl`.

The main risk the user named is that the tool gets built and then nobody uses
it. Their plan is to introduce it to every team member and back it with good,
written documentation. So ease of use and documentation are part of the
product, not extras.

## Mockups

The agreed flow is mocked up at https://claude.ai/artifact/3VwV4pPE5VEfj479G72VKW
(private to the user). The user approved it as-is. Component names in it are
examples.

## Context

- One self-hosted Mattermost server, **Team Edition v10.11.5**, with **one team
  per org**. Each org's team gets a status channel.
- Upgrading to Mattermost v11 is planned "at some point", date unknown. The
  plugin must work on 10.11 and use nothing newer.
- `statusctl`'s logic is in public Go packages: `model` (types, validation),
  `ops` (every change as a pure function `State → Result`), `store` (local dir
  or GitHub API, with conflict retry). The CLI is a thin layer in
  `internal/cli`.

## Two pieces of work, in order

### 1. statusctl: add Edit and Retract

`statusctl` today only appends updates. A mistaken post can be corrected only
by posting another update on top. The user decided mistakes must be fixable so
they disappear from the page.

- **Edit** — change the wording of an event's latest update in place.
  *(My call: edit covers the latest update's message and the event title, and
  for maintenance that hasn't started yet, its scheduled start and end, with
  the same end-after-start check as `maintenance new`. It does not change
  status or impact; those go through a normal update.)*
- **Retract** — remove an incident or maintenance window from the page
  entirely (a false alarm, or maintenance scheduled by mistake). The GitHub
  repo's git history keeps the record; nothing else does.
  *(My call: Retract is for catching mistakes early, so `ops` only allows it
  on events that are still open or that closed within the last 7 days.
  Retracting older events would quietly rewrite history and published uptime.
  The CLI can override this with an explicit flag; the plugin can't.)*
- `ops.ParseTime` gains the space-separated form `2026-11-10 22:00` alongside
  the existing `2026-11-10T22:00`, so the maintenance form's example works.
- Both are added to `ops` as pure functions like the existing ones, exposed in
  the CLI (e.g. `statusctl incident edit`, `statusctl incident retract`, and
  the maintenance equivalents — exact command shape is the builder's call),
  with `--json` and the existing exit codes.
- Retract deletes `incidents/<id>.json`. `ops.Result` currently only carries
  files to write and builds its commit message from the latest update, and
  `store/github.go` only writes blobs. Both need to support a deletion
  (GitHub's tree API takes `sha: null` for a removed path) and a sensible
  commit message for edit/retract. The local store needs the same.
- Edit and retract should still record who did it — at minimum in the commit
  message and commit author, since the edited/removed data itself no longer
  shows it.
- Release a new tagged `statusctl` version so the plugin can depend on it.

### 2. New repo: `Fab-IT-ApS/mattermost-plugin-statusctl`

Open source (MIT, like `statusctl`), separate repo next to `statusctl`. A Go
server plugin that imports `statusctl`'s `model`, `ops` and `store` packages,
so it writes to GitHub exactly the way the CLI does. It does not shell out to
the binary.

## How it behaves

### Where it works

- Installed once on the server. Configured by a system admin in the System
  Console with one entry per org: **team, status channel, GitHub repo, branch,
  token, public label**.
- `/status` only works in a configured status channel. The channel decides the
  org; nobody ever picks one. Anywhere else, it replies (ephemerally) that it
  only works in the status channel, and names it if the team has one.
- Tokens live in the plugin's configuration, never on anyone's laptop. They are
  never shown in channels or to non-admins. The token is the same kind the
  README describes: fine-grained, one repo, Contents read/write.
- How the per-org list is represented in the System Console is the builder's
  call (Mattermost plugin settings have no native "table" type; a structured
  long-text setting or a custom setting are both fine), as long as a system
  admin can add an org without editing files on the server.

### Who can post

- **Anyone who can see the status channel** can post. No approval step.
- Updates go live immediately. Safety comes from visibility and easy undo:
  every change is announced in the channel with Edit and Retract buttons.
- **Customers never see who posted.** Each org entry in the plugin config has
  a fixed **public label** (e.g. "Fab:IT Support"), and that label is what goes
  in `Update.Author`, which the public page prints on every timeline entry.
  (User's decision.)
- The real person is recorded in two places: the Mattermost card, and the git
  commit (as commit author name, or in the commit message). Today
  `store/github.go` uses `Update.Author` as the commit author name, so
  `statusctl` needs a way for a caller to pass a separate commit author. How is
  the builder's call; the CLI's behaviour (author = commit author) must not
  change.

### The form (main path)

`/status` with no arguments opens an interactive dialog. For a new incident it
asks three things, in plain words (see mockup step 02):

1. **What's wrong?** — the title.
2. **What is affected?** — one dropdown per component from that org's
   `status.yaml` (non-retired components only), with plain-word choices:
   *Working normally · Slow or degraded · Partly down · Down*, mapping to
   `operational`, `degraded`, `partial_outage`, `major_outage`. At least one
   component must be something other than "Working normally".
3. **Message to customers** — optional; blank means the default
   "We are investigating."

Status starts as Investigating and isn't asked for.

*(My call: how `/status` with no arguments lets the user choose between
"report a problem", "schedule maintenance" and "see current status" — e.g. a
first small dialog, or an ephemeral message with buttons — is the builder's
call. Reporting a problem should be the most prominent option.)*

### Typed commands (secondary path)

For people who prefer typing, `/status` subcommands mirror the CLI
(`/status incident new …`, `/status incident update …`, `/status maintenance
new …`, `/status show`, `/status list`, etc.), with Mattermost autocomplete.
Exact syntax is the builder's call; it should stay close to the CLI's so the
README's examples translate directly.

### The card in the channel

After every change, the plugin posts a card in the channel (mockup step 03):
who did it, the event title and status, the affected components with impact,
the message exactly as published, and a link to the org's page (`site.url`).
Buttons:

- **Post update** — opens a short form: new status, impact per component,
  message.
- **Resolve** — asks for a closing message (optional; default text as in the
  CLI), then resolves.
- **Edit** — opens the form pre-filled with the latest update's text and the
  title; saves in place.
- **Retract** — asks for confirmation, then removes the event from the page.

Maintenance cards get the maintenance equivalents (Start, Complete, Cancel,
Edit, Retract).

After a retraction, the plugin posts a short note in the channel (mockup step
04): what was retracted, who posted it, who retracted it, and that the record
stays in the repo's history. Customers see nothing.

*(My call: buttons on older cards act on the event's current state, not on the
state when the card was posted. If an action no longer applies — e.g. Resolve
on an already-resolved incident — the user gets a plain-word explanation.
Editing from an older card edits the current latest update, and the form shows
that text so there's no surprise.)*

### Maintenance

The maintenance form asks for title, affected components, start, end and an
optional message. Mattermost 10.11 has no reliable date picker in dialogs, so
start and end are plain text boxes pre-filled with an example such as
`2026-11-10 22:00`, interpreted in the org's timezone from `status.yaml`
(`ops.ParseTime` gains this form in step 1). If it can't read what was typed,
it says so in plain words and keeps the user's other input.

Maintenance never starts or completes by itself: someone presses **Start** and
**Complete** on the card. The plugin runs no timers and sends no reminders
(user's decision). The confirmation card after scheduling, and the user guide,
say this plainly, e.g. "This won't start by itself. Press Start when the work
begins." When the server
moves to v11, switching to the `datetime` dialog element is a small follow-up.

### Seeing current status

`/status show` (and an option from the bare `/status`) shows, ephemerally,
what the page currently says: overall status, each component, and open
incidents/maintenance with their cards' actions.

### Errors

Errors are explained in plain words for non-technical users, using
`statusctl`'s error categories: invalid input (say what to fix), write conflict
after retries (safe to try again), and auth/network failure (tell the user the
page wasn't updated and to tell an admin; log details server-side). The user
must always know whether the page was updated or not.

## Documentation (part of the deliverable)

In the plugin repo:

1. **User guide** — short, plain language, for everyone: how to report a
   problem, post an update, resolve, schedule maintenance, fix a mistake (edit /
   retract). Screenshots of the form and the card. Written so someone in sales
   can follow it during an outage.
2. **Admin guide** — installing the plugin, creating the GitHub token, adding
   an org (team, channel, repo, branch, token), upgrading.

`/status help` in Mattermost gives a brief summary and links to the user guide.

`statusctl`'s README gets the new edit/retract commands.

## Goals

- Anyone in the company can post an accurate status update in under a minute,
  from the status channel, without help.
- Every change is visible to the team in the channel, with who did it.
- Mistakes can be fixed or removed from the public page quickly.
- One plugin install serves every org on the server.

## Non-goals (first version)

- Announcing in Mattermost when someone updates a page from the CLI or GitHub
  directly.
- An approval step before publishing.
- Automatic starting/completing of maintenance, or reminders at its times.
- Changing an org's components, branding or site settings from Mattermost;
  that stays in `status.yaml`.
- Features that need Mattermost newer than 10.11, Enterprise, or Cloud.

## Anti-goals

- A tool only technical people are comfortable using. If the form needs the
  user to know statusctl's terms (impact names, IDs, ISO timestamps beyond the
  maintenance example), it has failed.
- Leaking tokens into channels, logs visible to users, or the repo.
- Staff names on the public page.
- Any doubt after submitting about whether the page actually changed.

## Left to the builder

- Plugin project scaffolding (Mattermost's plugin starter template is fine),
  build and release process, and CI, modelled on `statusctl`'s where sensible.
- Whether a webapp (frontend) part is needed at all; prefer server-only if the
  dialogs and interactive messages are enough.
- Exact command syntax, dialog wording beyond what's above, and card layout
  details.
- Test approach. The `ops` and `store` packages are already well tested;
  `store` has a fake GitHub for tests that the plugin can learn from.
