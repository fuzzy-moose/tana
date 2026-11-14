---
name: run-tana
description: Build, start, and drive Tana (local service, collector, and web UI) in an isolated stack with a fake Panda. Use when asked to run Tana, start the app or collector, take a screenshot of the UI, click through a page, or confirm a change works in the real app.
---

Start an isolated stack with `.claude/skills/run-tana/stack.sh`, then drive the UI with `.claude/skills/run-tana/driver.mjs` (headless Chromium through Playwright) or the HTTP API with `curl`. Paths are relative to the repo root.

The stack is a throwaway Postgres container, a fake Panda (`fake_panda.py`), the collector and the local service, all built from the working tree. It never reads `.env`.

## Prerequisites

Docker, Go, Node with `web/node_modules` installed, Python 3, and a system Chromium at `/usr/bin/chromium` (or set `CHROMIUM`). Install Playwright once for the driver:

```bash
mkdir -p /tmp/tana-run/node && npm i -s --prefix /tmp/tana-run/node playwright@1.59
```

## Run (agent path)

```bash
.claude/skills/run-tana/stack.sh start    # builds web/dist and both Go binaries, then starts everything
```

UI and local API: `http://127.0.0.1:8090`. Collector: `:8091` (token `runtoken`). Fake Panda: `:8092`. Postgres: `:5499`. Logs: `/tmp/tana-run/logs/`. Set `SKIP_BUILD=1` to reuse the last build.

Drive the UI by piping commands to the driver:

```bash
node .claude/skills/run-tana/driver.mjs <<'EOF'
nav #/collector/favorites
sidenav
check role=checkbox[name="Auto-download Category 3"]
click role=button[name="Save download categories"]
wait 700
sidenav
click [aria-label="Favorite sync"] >> role=button
watch 15
polls
shot favorites
size 390 800
nav #/
shot phone
EOF
```

Screenshots go to `/tmp/tana-run/shots/<name>.png`. Read them; a blank frame means the page did not load.

| command | what it does |
|---|---|
| `nav <hash>` | Open `http://127.0.0.1:8090/<hash>` and wait 2 s for the first data |
| `size <w> [h]` | New page at that viewport (phone: `size 390 800`); `nav` again after it |
| `click <selector>` / `check <selector>` | Playwright selector, e.g. `role=button[name="Sync all"]`, `text=Favorites page` |
| `wait <ms>` | Pause |
| `text <selector>` | Print element text, lines joined by ` \| ` |
| `sidenav` | Print the sidenav favorite sync quick action |
| `watch <s>` | Print the quick action each time it changes, for `s` seconds |
| `polls` | Print when `/favorites/status` requests went out since `nav` |
| `shot <name> [full]` | Screenshot, `full` for the whole page |
| `eval <js>` | Print the result of a page expression |

Other stack commands:

```bash
.claude/skills/run-tana/stack.sh status
.claude/skills/run-tana/stack.sh stop-collector                          # local API returns collector_unavailable
.claude/skills/run-tana/stack.sh start-collector
.claude/skills/run-tana/stack.sh restart-tana TANA_COLLECTOR_API_TOKEN=wrong   # collector_unauthorized
.claude/skills/run-tana/stack.sh restart-tana TANA_COLLECTOR_URL=              # collector_not_configured
.claude/skills/run-tana/stack.sh restart-tana                                  # back to normal
.claude/skills/run-tana/stack.sh stop                                          # stops processes and removes the container
```

Control the fake Panda with files of category numbers in `/tmp/tana-run/control/`, read on each request:

```bash
echo 5 > /tmp/tana-run/control/fail_parse   # category 5 syncs fail (unknown page layout)
echo 3 > /tmp/tana-run/control/fail_503     # category 3 gets HTTP 503 -> waiting_cooldown, retried later
rm /tmp/tana-run/control/fail_*             # clear
```

Call the API directly through the local service:

```bash
curl -s 127.0.0.1:8090/api/collector/favorites/status
curl -s -X POST -H 'Content-Type: application/json' -d '{}' 127.0.0.1:8090/api/collector/favorites/all/sync
```

## Run (human path)

`go run ./cmd/tana --dev` loads `.env`. Do not use it for agent checks; see the first gotcha.

## Test

```bash
cd web && npm run lint && npm test && npm run build
go test ./...
```

The Go tests start their own Postgres through testcontainers, so Docker must run.

## Gotchas

- **`.env` points at the production collector.** `TANA_COLLECTOR_URL` in `.env` is the real cloud collector, which syncs the user's real Panda account. `stack.sh` uses `env -i` and never `--dev`, so neither binary loads `.env`. Keep it that way.
- **Ports 8080 and 8081 are often taken** on this machine (other containers, including a deployed `tana` image on 8081). The stack uses 8090-8092 and 5499.
- **The collector calls the real sitemap by default.** `TANA_COLLECTOR_SITEMAP_URL` defaults to e-hentai; `stack.sh` points it at the fake Panda. Point every new upstream URL variable at the fake too.
- **Favorites requests are paced at one per 10 s.** A baseline sync of all ten categories takes about 100 s. Use `watch 120` or plan around it.
- **A favorites page must name the category.** The parser fails with "category controls missing" unless the page has a `favcat` option for the category, the 100-per-page setting, and "No favorites found" for an empty result. `fake_panda.py` serves exactly that; `fail_parse` drops it.
- **POST routes need `Content-Type: application/json`.** Without it the local service returns 415.
- **`polls` counts every status request.** On the Favorites page the page itself polls every 5 s in addition to the sidenav quick action. Measure the quick action rate on `#/`.
- **Playwright's bundled browsers may not match the npm version.** The driver launches the system Chromium (`executablePath`), so no `npx playwright install` is needed.

## Troubleshooting

- **`browserType.launch: Executable doesn't exist at ~/.cache/ms-playwright/...`**: Playwright looked for its own browser. Run the driver as committed, or set `CHROMIUM` to a Chromium binary.
- **A category stays "success" after `echo N > fail_parse`**: the fake Panda reads the file per request, and the collector had already fetched that category. Sync the category again: `curl -s -X POST -H 'Content-Type: application/json' -d '{}' 127.0.0.1:8090/api/collector/favorites/N/sync`.
