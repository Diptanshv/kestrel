# Decisions

Each entry records what was chosen, what it costs, and what would change it.

## Cookieless visitor identity

**Choice.** `visitor_id = first16(HMAC-SHA256(daily_salt, site_id || ip ||
user_agent))`. Each UTC day gets a fresh random salt, which is deleted after
2 days. IP and User-Agent are used in memory and never stored.

**Cost.** Unique visitors are only comparable within a UTC day. Someone who
visits on Monday and Tuesday counts twice (see "Multi-day uniques" below).
Two people behind one NAT with an identical browser build count once.

**Why.** No cookies and no persistent identifiers means no consent banner.
Once a salt is deleted, nobody can link a hash back to an IP or to another
day, including us.

## Sessions in memory, one instance

**Choice.** A map of `(site, visitor) → {session_id, last_seen}`, with a
30-minute inactivity window, a forced break at 00:00 UTC (the salt changes
there anyway), and eviction every minute. Session IDs are random 63-bit
integers, so restarts never reuse one.

**Cost.**
- A restart forgets open sessions: a visit spanning a deploy is counted as
  two.
- It pins the service to one instance.

**Why.** It is the cheapest correct option for a free-tier box. A Postgres
lookup per event would put database I/O back in the ingest path, which
Phase 6 removed. Redis is the scale-out path (see
[architecture.md](architecture.md)).

## Batched ingestion: drop under overload

**Choice.**
- Handlers enqueue onto a 10k channel without blocking.
- One goroutine flushes with `COPY` at 500 events or every 2 s.
- When the buffer is full, the event is dropped and counted; it is never
  queued behind the request.
- A failed `COPY` drops its batch rather than retrying.

**Cost.**
- A hard kill loses up to 2 s of events. `SIGTERM` drains them.
- A database outage loses events instead of backing up.

**Why.** Analytics must never slow the customer's page. `COPY` is 22×
faster than per-row inserts, and the handler's p99 fell from 39 ms to
1.9 ms (benchmarks.md). Retrying failed batches would let an outage fill
memory and then stall the request path.

## `text/plain` beacons

The tracker posts JSON as `text/plain`. That keeps the request a CORS "simple
request", so there is no preflight round trip, and `sendBeacon` works
everywhere. The server parses the body as JSON regardless of the content
type.

## Buckets in the configured time zone, not UTC

The design doc said "UTC only". The dashboard is used from India, though,
where UTC days start at 05:30. So stats and rollups both bucket in
`TIMEZONE` (default `Asia/Kolkata`):

- hourly rows start on the local hour;
- daily rows hold the local calendar date.

**Cost.**
- One time zone per deployment, not per site.
- The visitor salt still rotates at 00:00 UTC, mid-way through an IST day,
  so a person active at 04:00 and 07:00 IST counts as two visitors that day.
- Changing `TIMEZONE` later misaligns existing rollup rows.

Per-site zones are a stretch goal; they would need 15-minute buckets
(UTC+5:30 does not align to UTC hours).

## Retention with `DELETE`, not partitions

**Choice.** The rollup job runs `DELETE FROM events_raw WHERE ts < cutoff`
after each run.

**Why.** Raw data is at most about 48 hours of events, a few hundred
thousand rows. A range `DELETE` on the `(site_id, ts)` index plus autovacuum
handles that comfortably, and it avoids managing daily partitions (create
ahead, detach, drop) on a free-tier database.

**Would change if:** raw volume reached millions of rows per day. Then
dropping whole daily partitions would beat `DELETE` plus vacuum.

## Cardinality cap: top 50 per bucket, rest is `(other)`

Each rollup bucket keeps the 50 values with the most pageviews per
dimension and folds the rest into `(other)`, summing counters and merging
sketches. That bounds rollup size whatever a site, or an attacker, sends.
Long-tail values appear only within the 48 h raw window. The dashboard
explains `(other)` in its footnote and in a tooltip.

## HyperLogLog: precision 12, own implementation

**Choice.** `internal/hll` with p=12 (4,096 registers, about 1.6% standard
error). Sketches are stored sparse (delta-encoded register list) while that
is smaller, and dense (6-bit packed, 3,075 bytes) after that. Estimation uses
Ertl's improved estimator (2017).

**Why p=12.** Measured in `docs/benchmarks.md`:

- p=10 saves bytes only on busy rows (771 B versus 3,075 B dense). Its
  worst-case error of 7 to 9% shows up as visible day-to-day noise on a
  small dashboard.
- p=14 halves the error but costs 12 KB per busy row. Assume a busy site
  has about 20 dense rows per hour (the total, plus top pages, countries and
  browsers). Over 30 days of hourly retention that is 20 × 24 × 30 × 12 KB,
  about 177 MB for one site. At p=12 the same site needs about 44 MB.
  Against a 500 MB free tier, p=14 caps us at two busy sites.
- p=12 stays under about 3% worst case. Low-traffic rows, which are most
  rows, stay sparse and small: 24 to 200 bytes for up to 100 visitors.

**Why build it.** The sketch is about 300 lines with tests and a fuzzer. Doing
it in-house keeps the serialized format under our control; it is stored in
the database, so a library upgrade must not change it. It also avoids a
dependency for something this small. The fuzzer found one real bug: decoded
register values were not bounds-checked. `Unmarshal` now validates them.

**Hashing.** Visitor IDs are already HMAC-SHA256 output, so their first 8
bytes are uniformly random. They still pass through a splitmix64 finaliser
so the sketch stays correct for any other input.

**Would change if:** traffic per site grew enough that dense rows dominate
storage (move hourly rows for low-value dimensions to p=10), or if exact
small counts mattered (keep an exact set below a few hundred visitors).

## Multi-day uniques are the sum of daily uniques

Visitor IDs are `HMAC(daily_salt, site || ip || ua)`, and the salt rotates at
00:00 UTC and is deleted after 2 days. The same person therefore has
unrelated IDs on different days, and nothing can link them, not even us.
Merging daily sketches across days would not help, because the IDs differ.
A 30-day "unique visitors" figure is the sum of 30 daily figures, and the UI
says so. This is the trade-off other cookieless tools accept.

## Rollup frontier and backfills

The rollup job remembers how far it has rolled (`rollup_state`), and each run
recomputes only the last 2 hours before that point. Raw retention never
deletes past the frontier, so nothing it has not yet rolled up is lost.

This relies on one invariant: an event's `ts` is the server's receive time,
so new rows never land far behind the frontier. Backdated inserts (imports,
the demo seeder) break that invariant. On the next run they would sit behind
the frontier, never be rolled up, and be deleted by retention. During
development a preview seed lost 4,821 rows exactly this way.

Resetting `rollup_state` is not the fix either. The next run would then
rebuild every hour back to the oldest raw event and delete other sites'
hourly rows whose raw events are already gone. So backdated data goes
through `rollup.Job.Backfill(site, from)`, which:

- rebuilds hours and days for that one site only;
- takes the same advisory lock;
- leaves the frontier alone.

`TestBackfillIsSiteScoped` covers it.

## Filters only cover the raw window

Rollup rows store each dimension on its own: (hour, page) and (hour,
country) are separate rows. That answers "top pages" and "top countries",
but not "top pages for visitors from Germany". So filters run against raw
events, which are kept 48 hours, and a filtered dashboard always shows
exactly that window, with a label saying so. Composite rollups for a few
popular pairs are the stretch goal if longer filters are needed.
