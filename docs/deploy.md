# Deploying and operating Kestrel

The target is free-tier hosting. The pieces are:

| Piece | Where | Config in repo |
|-------|-------|----------------|
| API (Go, one container) | Render free web service, or any container host | `Dockerfile`, `deploy/render.yaml` |
| Postgres | Neon free tier (or Supabase) | none, just `DATABASE_URL` |
| Dashboard (static SPA) | Netlify | `web/netlify.toml` |
| CI and deploys | GitHub Actions | `.github/workflows/ci.yml` |
| Keep-awake and rollup schedule | GitHub Actions cron | `.github/workflows/ops.yml` |

> Free-tier limits and terms change often. Verify each provider's current
> limits before relying on them, and record the date you checked.

## Why the dashboard proxies the API

The browser has to see a single origin. The session cookie is
`SameSite=Lax`, and the dashboard reads the CSRF token from
`document.cookie`; neither works if the dashboard is on `*.netlify.app` and
the API is on `*.onrender.com`. Netlify therefore rewrites `/api/*` and
`/script.js` to the API (`web/netlify.toml`). The browser only ever talks to
the dashboard's domain. The tracker snippet points at that domain too, which
also helps a little against ad-blockers.

Each proxy appends one `X-Forwarded-For` hop. With Netlify in front of
Render's load balancer there are two, so set `TRUSTED_PROXY_HOPS=2`. If this
number is wrong, per-IP rate limits and visitor hashing either key on a proxy
address or trust a forged one (see `docs/security.md`).

## One-time setup

1. **Database (Neon).** Create a project and copy the **direct** connection
   string, not the `-pooler` one, and keep `sslmode=require`. The rollup job
   uses a session-level advisory lock and pgx caches prepared statements;
   both break behind a transaction-mode pooler. Migrations run automatically
   when the API starts.
2. **API (Render).** Create a web service from this repository with the
   Docker runtime, or apply `deploy/render.yaml` as a blueprint. Set the
   environment below. Turn auto-deploy off: CI deploys after the tests pass.
   Copy the service's **deploy hook URL**.
3. **Dashboard (Netlify).** Create a site with base directory `web`. In
   `web/netlify.toml`, replace `kestrel-api.onrender.com` with your API host.
   Copy the **site ID** and create a **personal access token**.
4. **GitHub.** In the repository settings:
   - Secrets: `RENDER_DEPLOY_HOOK_URL`, `NETLIFY_AUTH_TOKEN`,
     `NETLIFY_SITE_ID`, `ROLLUP_SECRET`.
   - Variable: `API_URL` (for example `https://kestrel-api.onrender.com`).
   - Optionally, an environment named `production` with required reviewers,
     to gate deploys.

Each deploy step is skipped when its secrets are missing, so CI stays green
before hosting is set up.

## API environment

| Variable | Production value | Notes |
|----------|------------------|-------|
| `DATABASE_URL` | Neon direct URL | Required |
| `SESSION_SECRET` | long random string | Required; rotating it logs everyone out |
| `COOKIE_SECURE` | `true` | Cookies only over HTTPS |
| `TIMEZONE` | `Asia/Kolkata` | Buckets for stats and rollups. Changing it later misaligns existing rollup rows |
| `TRUSTED_PROXY_HOPS` | `2` | Netlify proxy plus host load balancer |
| `ROLLUP_SECRET` | random; same as the GitHub secret | Enables `POST /internal/rollup` |
| `METRICS_TOKEN` | random | Enables `GET /metrics` (Bearer) |
| `ROLLUP_INTERVAL` | `15m` (default) | In-process ticker; the cron covers sleeps |
| `INGEST_IP_RATE` / `_BURST` | `10` / `50` (default) | Per client IP |
| `INGEST_SITE_RATE` / `_BURST` | `200` / `1000` (default) | Per site |
| `INGEST_BUFFER_SIZE`, `INGEST_BATCH_SIZE`, `INGEST_FLUSH_INTERVAL` | defaults | See `docs/benchmarks.md` |
| `GEOIP_DB_PATH` | set by the image | DB-IP Lite is downloaded at build time; country is empty if that failed |
| `PORT` | set by the host | |

`ENABLE_DEMO` must stay unset in production.

## The release flow

1. Open a pull request. `ci.yml` runs `go vet`, the test suite against a
   Postgres service (with `-race`), golangci-lint, the web lint and build,
   and a Docker build.
2. Merge to `main`. The same checks run again. If they pass, the `deploy`
   job triggers the Render deploy hook and publishes the dashboard with the
   Netlify CLI.
3. On `SIGTERM` the API stops accepting requests, drains the ingest buffer
   into Postgres, and exits. A deploy loses no accepted events; a hard kill
   loses at most `INGEST_FLUSH_INTERVAL` (2 s).

## Keeping a sleeping host useful

Free web services sleep after a period without traffic. `ops.yml` runs two
schedules:

- **Every 10 minutes:** `GET $API_URL/healthz`. This keeps the instance
  warm, so the first beacon after a quiet spell is not lost to a cold start,
  and doubles as an uptime check. cron-job.org works too, if you prefer it
  outside GitHub.
- **Hourly at :07:** `POST $API_URL/internal/rollup`. If the host slept
  through the in-process ticker, this catches up. The job resumes from
  `rollup_state` and is idempotent, so late or duplicate runs are harmless.
  A `409` means a run was already in progress.

GitHub disables scheduled workflows in repositories with no activity for 60
days. A push re-enables them.

## Dogfooding

1. Sign in to the deployed dashboard and add your blog's domain, for example
   `yourblog.com`.
2. Paste the snippet it shows into your site's `<head>`:

   ```html
   <script defer src="https://<your-dashboard>/script.js" data-domain="yourblog.com"></script>
   ```

3. Load a page. It shows up in the realtime badge within a second or two.
4. To record goals, call `kestrel.track('signup')`. To track calls made
   before the script loads, add this stub first:

   ```html
   <script>window.kestrel=window.kestrel||{q:[],track:function(){this.q.push(arguments)}}</script>
   ```

## Watching it run

- `GET /metrics` (Bearer `METRICS_TOKEN`) exposes:
  - `events_received_total`
  - `events_dropped_total{reason}`
  - `batch_flush_duration_seconds`
  - `buffer_depth`
  - `rollup_duration_seconds`
  - `rollup_last_success_timestamp`
  - `http_request_duration_seconds{route}`

  Grafana Cloud's free tier can scrape it.
- Alert on two conditions:
  - `rollup_last_success_timestamp` more than 2 hours old. Raw events expire
    after 48 hours, but they are never deleted ahead of the rollup frontier,
    so a stalled job grows the database rather than losing data.
  - Any sustained `events_dropped_total{reason="buffer_full"}` or
    `{reason="write_error"}`.
- Logs are JSON on stdout and record no IPs.

## Local equivalents

```bash
docker compose up --build   # db + api + dashboard on http://localhost:3000
make up && make run         # db only, API from source on :8080
make web-dev                # Vite on :5173, proxying the API
```
