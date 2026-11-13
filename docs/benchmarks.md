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
