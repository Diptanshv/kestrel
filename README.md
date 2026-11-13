# Kestrel

Kestrel is cookieless, privacy-first web analytics in the vein of Plausible.
You paste one `<script>` tag, and the dashboard shows visitors, pageviews,
visits, bounce rate, top pages and sources, countries and devices, goals,
and a live visitor count. It uses no cookies, stores no IPs and builds no
cross-day profiles, so the sites it tracks need no consent banner.

It is a Go API, a Postgres database and a React dashboard, designed to run
on free-tier hosting (about 512 MB RAM and a 500 MB database).

![Dashboard: 30 days of demo traffic](docs/screenshots/dashboard.jpg)

> **Live demo:** not deployed yet. [docs/deploy.md](docs/deploy.md) has
> the Render + Neon + Netlify setup. Locally,
> `docker compose up --build && make seed` serves the same demo at
> `http://localhost:3000/share/demo`.

## Numbers

| | |
|-|-|
| Ingest latency, 0.5 CPU, 2,600 events/s sustained | **p99 8.1 ms** (target < 10 ms); p50 0.6 ms |
| Ingest throughput, batched vs. synchronous insert | 61k vs 2.2k req/s on a laptop; `COPY` is 22× faster per row |
| Events lost under 480k-event load | **0** (accepted = written) |
| Storage | 21 MB per 100k raw events (kept 48 h); about 5 MB/site/month in hourly rollups; about 3 MB/site/year in daily rollups |
| Unique-visitor accuracy after raw data is gone | within about 1–2% of `COUNT(DISTINCT)` (HyperLogLog, p=12) |
| Tracker size | 0.8 KB gzipped |

Methods and raw results are in [docs/benchmarks.md](docs/benchmarks.md).

## Run it

**Everything in Docker:**

```bash
docker compose up --build        # Postgres, API (:8080), dashboard (:3000)
```

Open <http://localhost:3000> and register. Then:

1. Add a site, for example `localhost`.
2. Paste the snippet it shows into any page. With `ENABLE_DEMO=true`,
   which compose sets, <http://localhost:8080/demo.html> is ready-made.

To populate a demo site with 30 days of realistic traffic:

```bash
DATABASE_URL="postgres://pulse:pulse@localhost:5433/pulse?sslmode=disable" make seed
# → http://localhost:3000/share/demo
```

**From source** (Go 1.27, Node 22):

```bash
cp .env.example .env               # set DATABASE_URL and SESSION_SECRET
make up                            # Postgres only, on :5433
make run                           # API on :8080; migrations run on boot
make web-install && make web-dev   # dashboard on :5173, proxying the API
make test                          # Go tests against a separate pulse_test DB
```

## How it works

```
tracker.js ──beacon──▶ POST /api/event ──▶ rate limit · validate · enrich ──▶ channel
                                 (202 in ~1 ms, no DB I/O)                       │
                                                                        batcher (COPY, 2 s)
                                                                                 ▼
 dashboard ◀── stats API ◀── events_raw (48 h) ──rollup job──▶ hourly (+HLL, 30 d) ──▶ daily (forever)
```

- **Cookieless identity.** Each visitor is
  `HMAC(daily_salt, site ‖ ip ‖ user_agent)`. The salt rotates at midnight
  UTC and is deleted after 2 days. IP and User-Agent never touch disk.
- **Non-blocking ingest.**
  - Handlers enqueue onto a buffered channel and return `202`.
  - One goroutine bulk-loads with `COPY`.
  - Under overload, events are dropped and counted, never queued behind
    the customer's page.
  - Shutdown drains the buffer.
- **Sessions.** Visits are tracked in memory with a 30-minute inactivity
  window. They drive visits, bounce rate and visit duration.
- **Rollups.** An idempotent job runs under a Postgres advisory lock. It
  rebuilds hourly buckets with the top 50 values per dimension, plus an
  `(other)` row, and merges them into days. A ticker runs it in-process,
  and an external cron covers hosts that sleep.
- **HyperLogLog.** Each hourly row carries a mergeable sketch, so daily
  unique counts survive after raw events expire. The implementation is in
  [internal/hll](internal/hll), written from scratch and fuzzed.
- **Hardening.** Defences include:
  - token buckets per IP and per site;
  - trusted-proxy-aware client IPs;
  - strict UTF-8 and length validation, so one bad beacon cannot poison a
    `COPY` batch;
  - CSRF protection and secure cookies;
  - token-guarded `/metrics` and `/internal/rollup`.

The full diagrams and request lifecycle are in
[docs/architecture.md](docs/architecture.md).

## Dashboard features

- **Range.** Today, 7 or 30 days, with change badges against the previous
  period.
- **Click to filter.** Any page, referrer, country, device, browser or OS
  filters the dashboard. Filters cover the last 48 hours, and the page says
  so.
- **Goals and custom events.** Track them with `kestrel.track('signup')`;
  the dashboard shows conversion rates. Calls made before the script loads
  are queued and replayed.
- **Realtime badge.** Visitors in the last 5 minutes.
- **Public share links.** Read-only and revocable.
- **Dark mode and mobile.** Dark mode follows the system, and the layout
  adapts to mobile.

![Breakdowns and goals](docs/screenshots/breakdowns.jpg)

## Design decisions

[docs/decisions.md](docs/decisions.md) explains each trade-off: what was
chosen, what it costs, and what would change it.

- Cookieless hashing, and why multi-day uniques are a sum of daily ones.
- In-memory sessions on a single instance.
- Dropping events under overload.
- HyperLogLog precision 12, built in-house.
- Retention with `DELETE` rather than partitions.
- Buckets in the deployment's time zone.
- Filters limited to the raw window.
- Backfills that are safe for other sites.

The threat model (spam, oversized payloads, bad origins, spoofed IPs) is in
[docs/security.md](docs/security.md).

## Repository layout

```
cmd/server        API binary: wiring, graceful shutdown
cmd/seed          demo traffic generator
internal/
  httpapi         routes, middleware, handlers
  ingest          payload validation, URL normalisation
  enrich          salts, visitor hash, UA, GeoIP
  sessions        in-memory session tracker
  batch           channel + COPY batcher
  rollup          hourly/daily rollups, backfill, retention
  hll             HyperLogLog sketch
  stats           raw-vs-rollup query planning
  ratelimit       token buckets
  metrics         Prometheus collectors
  store           sqlc-generated queries
migrations/       schema (golang-migrate)
queries/          SQL for sqlc
web/              React + TypeScript + Vite dashboard
loadtest/         k6 script
docs/             architecture, decisions, benchmarks, security, deploy
```

## Privacy and licences

Country lookup uses the DB-IP Lite database (CC BY 4.0), which the Docker
image downloads at build time. Without it, country is simply left empty.
