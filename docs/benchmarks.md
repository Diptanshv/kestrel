# Benchmarks

All numbers are from a laptop (Apple M1 Pro, 10 cores), Postgres 16 in Docker
on the same machine, Go 1.27. They show relative cost, not production
capacity. Measure again on the target instance before quoting them.

## Phase 6: synchronous versus batched ingestion

### Write strategy: 500 events per call

```
TEST_DATABASE_URL=... go test ./internal/batch -run '^$' -bench BenchmarkWrite -benchtime 3s
```

| Strategy | Time per 500 events | Throughput |
|----------|--------------------:|-----------:|
| One `INSERT` per row (Phase 3 path) | 200 ms | ~2,500 events/s |
| `pgx.CopyFrom` (one `COPY` per batch) | 9 ms | ~55,700 events/s |

`COPY` is about **22x** faster for the same rows. Per-row inserts pay one
network round trip and one commit for every event. `COPY` streams the batch
in one statement and commits once.

### End to end: `POST /api/event` under concurrency

```
KESTREL_BENCH=1 TEST_DATABASE_URL=... go test ./internal/httpapi -run TestIngestLatency -v
```

32 concurrent clients, 8,000 beacons, distinct User-Agents so visitor hashing
does real work. Every accepted event was in `events_raw` afterwards (8,000 of
8,000 rows in both modes).

| Mode | Throughput | p50 | p95 | p99 | max |
|------|-----------:|----:|----:|----:|----:|
| Sync: insert in the handler | 2,214 req/s | 9.9 ms | 30.0 ms | 39.2 ms | 76 ms |
| Batched: channel, then `COPY` | 61,020 req/s | 0.32 ms | 1.0 ms | 1.9 ms | 34 ms |

The batched path meets design requirement N1 (under 10 ms p99) with a wide
margin. Sync p99 grows with load because every request waits for its own
commit. Batched latency stays flat because the handler does no database work:

- **Site lookup** is served from an in-process cache (1 minute TTL; unknown
  domains are cached for 10 seconds). Before the cache, the batched path's
  p99 was still 34 ms, because every request ran a `SELECT` against `sites`.
- **Salt** is cached per UTC day by `enrich.SaltStore`.
- **The write** is a non-blocking channel send.

The batched max (34 ms) is a GC or scheduler outlier from running the client,
server and database on one machine. p99 is the figure to watch.

### Trade-offs this buys

- **Bounded loss on hard kill.** Up to `INGEST_FLUSH_INTERVAL` (2 s) of events
  live only in memory. `SIGTERM` drains them (`TestGracefulStopLosesNothing`);
  `SIGKILL` does not.
- **Drops under overload.** When the 10k buffer is full, events are dropped
  and counted in `events_dropped_total{reason="buffer_full"}`. The handler
  still answers 202 because the tracker cannot act on an error.
- **A failed `COPY` drops the whole batch**
  (`events_dropped_total{reason="write_error"}`). Retrying would let a
  database outage back up into the request path.

## Phase 9: HyperLogLog uniques

### Error versus cardinality (sketch only)

```
KESTREL_BENCH=1 go test ./internal/hll -run TestAccuracyReport -v
```

Mean and worst absolute relative error over 20 trials per point. Size is the
serialized sketch, which the encoder keeps sparse while sparse is smaller.

| Distinct visitors | p=10 mean / max | p=12 mean / max | p=14 mean / max | p=12 size |
|------------------:|----------------:|----------------:|----------------:|----------:|
| 10 | 0.00% / 0.00% | 0.00% / 0.00% | 0.00% / 0.00% | 29 B |
| 100 | 2.10% / 5.00% | 0.60% / 3.00% | 0.35% / 2.00% | 198 B |
| 1,000 | 2.05% / 6.90% | 0.83% / 2.00% | 0.49% / 1.60% | 1,534 B |
| 5,000 | 1.83% / 4.28% | 1.02% / 3.04% | 0.41% / 1.14% | 3,075 B |
| 10,000 | 2.54% / 7.07% | 1.10% / 3.05% | 0.57% / 1.32% | 3,075 B |
| 100,000 | 2.81% / 5.48% | 1.16% / 2.23% | 0.56% / 1.57% | 3,075 B |
| 1,000,000 | 2.58% / 9.25% | 1.20% / 3.13% | 0.75% / 1.68% | 3,075 B |

Dense sizes are 771 B (p=10), 3,075 B (p=12) and 12,291 B (p=14). Error
stays flat from 100 to 1M visitors because Ertl's estimator has no
small-range or large-range switchover to bias it.

### Rollup uniques versus `COUNT(DISTINCT)`

```
KESTREL_BENCH=1 TEST_DATABASE_URL=... go test ./internal/rollup -run TestUniquesAccuracyReport -v
```

One synthetic day, two pageviews per visitor at random times. The daily row
is built by merging 24 hourly sketches. It is compared with
`count(DISTINCT visitor_id)` over the same raw rows.

| Exact | HLL (daily row) | Error | Avg hourly `_total` sketch | Rollup run |
|------:|----------------:|------:|---------------------------:|-----------:|
| 100 | 98 | -2.00% | 24 B | 20 ms |
| 1,000 | 1,007 | +0.70% | 163 B | 24 ms |
| 10,000 | 9,900 | -1.00% | 1,308 B | 138 ms |
| 50,000 | 49,400 | -1.20% | 3,075 B | 623 ms |
| 100,000 | 100,850 | +0.85% | 3,075 B | 1,182 ms |

Merging 24 hourly p=12 sketches and estimating takes 0.44 ms
(`BenchmarkMerge24Hourly`). Adding a visitor takes 6 ns (`BenchmarkAdd`).

## Phase 13: load test (k6)

```
make loadtest                      # or: k6 run loadtest/ingest.js
```

**Setup.** The API runs in its production image, capped with
`docker run --cpus 0.5 --memory 512m` to approximate a free-tier instance.
Postgres 16 is a separate, uncapped container, and k6 runs in Docker on the
same laptop. Rate limits were raised, because k6 sends from one IP to one
site.

**Load.** Each run ramps for 30 s to the peak, holds it for 60 s, then ramps
down. Every beacon carries a unique User-Agent, so each one is a new
visitor: the hash, session and cache paths all do real work.

| Peak target | Achieved (whole run) | p50 | p95 | p99 | Failed | Accepted = written |
|------------:|---------------------:|----:|----:|----:|-------:|:------------------:|
| 2,000 req/s | 1,615 req/s avg | 0.55 ms | 1.4 ms | **4.5 ms** | 0 | 161,499 = 161,499 |
| 2,600 req/s | 2,095 req/s avg | 0.57 ms | 2.0 ms | **8.1 ms** | 0 | yes |
| 4,000 req/s | 3,198 req/s avg (k6 dropped 1,652 iterations) | 0.72 ms | 36 ms | 79 ms | 0 | 481,346 = 481,346 |

"Achieved" averages over the ramps too, so the sustained rate during the
hold was the peak target. Results:

- **Capacity.** Half a CPU sustains **about 2,600 events/s within the
  10 ms p99 target** (design requirement N1). Past that, CPU saturates and
  latency climbs, but nothing is lost: every accepted event was written.
  Buffer drops would only start once the 10k buffer filled, and none
  occurred. Memory peaked at about 100 MB of the 512 MB.
- **Default limits.** The defaults admit 200 events/s per site and 10/s per
  IP. The pipeline has more than ten times the headroom of one busy site.

## Phase 13: storage per 100k events

```
DATABASE_URL=... go run ./cmd/seed -visitors 1600 -seed 7   # 100,342 events over 30 days
```

Measured with `pg_total_relation_size`, which includes indexes, after
`VACUUM ANALYZE`. The seeded traffic is 30 days of realistic visits: about
2.7 pageviews per visit, 10 entry pages and 9 referrers.

| Table | Rows | Size | Per 100k events |
|-------|-----:|-----:|----------------:|
| `events_raw` | 100,342 | 21.4 MB | **21.4 MB** (about 224 B/row with index) |
| `rollup_hourly` (30 days) | 27,257 | 5.1 MB | 5.1 MB |
| `rollup_daily` (30 days) | 1,380 | 0.25 MB | 0.25 MB |

Every hourly sketch stayed sparse at this traffic level: 29 bytes on average,
and no dense rows.

**What this means for a 500 MB database:**

- **Raw events** are kept for 48 hours, so at 100k events/day their steady
  state is about 43 MB.
- **Hourly rollups** are kept for 30 days. Their size grows with distinct
  values, not with traffic: the top-50 cap bounds each dimension. That is
  about 5 MB per site at this shape.
- **Daily rollups** are kept forever, at about 3 MB per site-year.

A site at 100k events/day therefore fits in well under 100 MB, and the
remaining space holds many quieter sites.
