# mkcr

A CLI for creating and listing ServiceNow Standard Change Requests without touching the UI.

Authenticates via a headless browser session (Microsoft SSO + MFA supported), then talks directly to the ServiceNow REST API.

## Installation

### Homebrew

```bash
brew tap eimarfandino/tap
brew install mkcr
```

### Download binary

Download the latest binary for your platform from the [releases page](../../releases), extract it, and put `mkcr` somewhere on your `$PATH`.

### Build from source

```bash
git clone https://github.com/eimarfandino/snow-cli.git
cd snow-cli
go build -o mkcr .
```

## Quick start

```bash
mkcr config   # one-time setup
mkcr login    # authenticate (re-run when session expires)
mkcr create --message "Deploy new ingress controller"
mkcr list
```

## Setup

```bash
mkcr config
```

Interactive prompt that saves your defaults to `~/.mkcr/config.json`. Re-run at any time to update.

| Field | Description |
|---|---|
| ServiceNow instance | Hostname only, e.g. `yourcompany.service-now.com` |
| Standard Change template sys_id | sys_id of the Standard Change template to clone |
| Configuration item (cmdb_ci) | Display name of the CI, e.g. `My Application [Production]` |
| Configuration item sys_id | sys_id of the CI — find it in the URL when opening the CI record in ServiceNow |
| Assignment group | e.g. `MY TEAM` |
| Assigned to | Full name as it appears in ServiceNow, e.g. `Jane Doe` |

## Authentication

```bash
mkcr login
```

Launches a headless Firefox browser, completes the SSO/MFA flow, and saves the session to `~/.mkcr/session.json`. You only need to re-run this when your session expires.

```bash
mkcr login --show-browser   # watch the login flow (useful for debugging)
```

## Commands

### `mkcr create`

Create a Standard Change Request from your configured template.

```bash
# No scheduled dates
mkcr create --message "Deploy new ingress controller"

# With a scheduled window
mkcr create --message "Deploy new ingress controller" --date 25-09-2026T10:00 --duration 1h30m
```

| Flag | Required | Description |
|---|---|---|
| `--message` | yes | Change description |
| `--date` | no | Scheduled start in `DD-MM-YYYYTHH:MM` format — must pair with `--duration` |
| `--duration` | no | Length of the change window, e.g. `1h`, `2h30m` — must pair with `--date` |

On success, prints the CR number and a direct link:

```
CHG0000001
https://yourcompany.service-now.com/nav_to.do?uri=change_request.do%3Fsys_id%3D...
```

### `mkcr list`

List Standard Change Requests for your configured CI.

```bash
mkcr list
```

By default shows all open (non-closed, non-cancelled) CRs, ordered by planned start date descending.

```
NUMBER      STATE  START DATE            END DATE              DESCRIPTION                               URL
------      -----  ----------            --------              -----------                               ---
CHG0000001  New    25-09-2026 10:00:00   25-09-2026 11:30:00   Deploy new ingress controller             https://yourcompany.service-now.com/...
CHG0000002  New    18-09-2026 14:00:00   18-09-2026 15:00:00   Rotate TLS certificates                   https://yourcompany.service-now.com/...
```

| Flag | Description |
|---|---|
| `--state` | Filter by one or more states, comma-separated |
| `--debug` | Print the raw API request URL and response body |

Valid state values: `new`, `assess`, `authorize`, `scheduled`, `implement`, `review`, `closed`, `cancelled`.

```bash
mkcr list --state scheduled
mkcr list --state new,scheduled
mkcr list --debug
```

## How it works

1. `mkcr login` saves a Playwright browser storage state (cookies + local storage) to `~/.mkcr/session.json`.
2. On each `create` or `list`, a headless Firefox instance loads that session and captures the `x-usertoken` header from an outgoing request, along with the relevant session cookies.
3. Those credentials are forwarded to the ServiceNow REST API directly — no browser UI, no manual copy-paste.

If the session has expired, `mkcr` automatically re-runs the login flow before retrying.
