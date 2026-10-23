# statusctl

A status page with no server and no database. Each organisation's status lives
as JSON files in its own GitHub repository; Netlify renders them into a static,
JavaScript-free site. `statusctl` posts updates through the GitHub API and
renders the site.

## Install

Download the binary for your platform from the [releases page](https://github.com/Fab-IT-ApS/statusctl/releases) and check
it against `SHA256SUMS`, or build from source with `go build ./cmd/statusctl`.

## Configure

`~/.config/statusctl/config.yaml` (macOS: `~/Library/Application Support/statusctl/config.yaml`,
Windows: `%AppData%\statusctl\config.yaml`):

```yaml
default_profile: acme
profiles:
  acme:
    repo: ORG/status-acme
    branch: main
    token_cmd: pass show github/status-acme
    author: Your Name          # optional; defaults to git config user.name
```

There is no field for a literal token. Use `STATUSCTL_GITHUB_TOKEN` or `token_cmd`.
The token should be a fine-grained token for that one repository with
**Contents: read and write** and nothing else.

## Post updates

```sh
statusctl incident new "Elevated API latency" -c api=degraded -m "Investigating."
statusctl incident update 2026-10-23-elev --status identified -c api=major_outage -m "DB failover."
statusctl incident resolve 2026-10-23-elev -m "Recovered."

statusctl maintenance new "DB upgrade" -c db --start 2026-11-02T22:00 --end 2026-11-03T00:00
statusctl maintenance start 2026-11-02-db
statusctl maintenance complete 2026-11-02-db

statusctl status          # current derived status
statusctl list --open     # open incidents and maintenance
```

Impacts: `operational`, `degraded`, `partial_outage`, `major_outage`.
Messages are Markdown (`-m`, `-F file`, `-F -` for stdin, or `$EDITOR`).
IDs accept any unique prefix. `--profile` selects an organisation.

## Automation

Every command takes `--json` and never prompts without a terminal.

| Exit | Meaning |
|---|---|
| 0 | ok |
| 1 | unexpected error |
| 2 | invalid input or invalid repository data |
| 3 | write conflict, retries exhausted (safe to retry) |
| 4 | authentication or network failure |

## Onboard an organisation

1. Create an empty GitHub repository, e.g. `ORG/status-globex`, and protect `main`
   (no force-push, no deletion).
2. `statusctl init --dir status-globex`, then edit `status.yaml` (name, URL,
   timezone, components, branding) and run `statusctl validate --dir status-globex`.
3. Commit and push.
4. In Netlify, add a site from that repository. Build settings come from `netlify.toml`.
   Add the custom domain.
5. In Netlify, create a build hook, and store its URL as the repository secret
   `NETLIFY_BUILD_HOOK` (used by the daily rebuild workflow).
6. Create a fine-grained token for the repository (Contents: read and write)
   and add a profile for it.

## Preview locally

```sh
statusctl render --dir status-globex --out /tmp/site
python3 -m http.server -d /tmp/site   # any static file server works
```

## Upgrade an organisation's page

Change `STATUSCTL_VERSION` and `STATUSCTL_SHA256` in its `netlify.toml`, taking
both from the release's `SHA256SUMS`, and commit.

## License

MIT. See [LICENSE](LICENSE).
