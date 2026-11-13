# Decisions

Each entry records what was chosen, what it costs, and what would change it.

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
