# mkcr

A CLI for creating and listing ServiceNow Standard Change Requests without touching the UI.

## Installation

Download the latest binary for your platform from the [releases page](../../releases), extract it, and put `mkcr` somewhere on your `$PATH`.

Or build from source:

```bash
git clone https://github.com/eimarfandino/snow-cr-cli.git
cd snow-cr-cli
go build -o mkcr .
```

## Setup

Run once to configure your ServiceNow instance and defaults:

```bash
mkcr config
```

You will be prompted for:

| Field | Description |
|---|---|
| ServiceNow instance | e.g. `now.example-instance.com` |
| Standard Change template sys_id | The sys_id of the Standard Change template to use |
| Configuration item (cmdb_ci) | Display name of the CI, e.g. `Kubernetes Platform [Production]` |
| Configuration item sys_id | sys_id of the CI (find it in the URL when opening it in ServiceNow) |
| Assignment group | e.g. `AWS ENABLEMENT` |
| Assigned to | Full name as it appears in ServiceNow, e.g. `Jane Doe` |

Config is saved to `~/.mkcr/config.json`.

## Authentication

```bash
mkcr login
```

Opens a headless browser, performs SSO login (Microsoft Authenticator MFA supported), and saves the session to `~/.mkcr/session.json`. You only need to re-run this when your session expires.

Use `--show-browser` to watch the login flow for debugging:

```bash
mkcr login --show-browser
```

## Commands

### `mkcr create`

Create a Standard Change Request.

```bash
# Minimal — no scheduled dates
mkcr create --message "Deploy new ingress controller"

# With a scheduled window
mkcr create --message "Deploy new ingress controller" --date 25-09-2026T10:00 --duration 1h30m
```

| Flag | Description |
|---|---|
| `--message` | Change description (required) |
| `--date` | Scheduled start, format `DD-MM-YYYYTHH:MM` (optional, must pair with `--duration`) |
| `--duration` | Length of the change window, e.g. `1h`, `2h30m` (optional, must pair with `--date`) |

On success, prints the CR number and a direct link:

```
CHG0128506
https://now.example-instance.com/nav_to.do?uri=change_request.do%3Fsys_id%3Dfb75009c...
```

### `mkcr list`

List open Standard Change Requests matching your config.

```bash
mkcr list
```

By default shows all non-closed, non-cancelled CRs for your configured CI, ordered by planned start date descending.

```
NUMBER      STATE  START DATE            END DATE              DESCRIPTION                               URL
------      -----  ----------            --------              -----------                               ---
CHG0128506  New    21-09-2026 10:00:00   21-09-2026 11:00:00   Deploy new ingress controller             https://now.example-instance.com/...
CHG0128333  New    14-09-2026 15:00:00   15-09-2026 08:00:00   Deploy new ingress controller             https://now.example-instance.com/...
```

Filter by state:

```bash
mkcr list --state scheduled
mkcr list --state new,scheduled
```

Valid states: `new`, `assess`, `authorize`, `scheduled`, `implement`, `review`, `closed`, `cancelled`.
