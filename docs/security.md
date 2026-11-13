# Security and abuse model

Kestrel has two very different surfaces:

- **`POST /api/event`** is public by design. Any browser on any site can call
  it, and anyone with `curl` can too.
- **The dashboard API** sits behind a session cookie and CSRF protection.

The goals are to keep the service up, keep the data honest enough to be
useful, and never hold data that identifies a person. Perfect data is not a
goal: an analytics beacon cannot authenticate its sender.

## Assets

| Asset | Why it matters |
|-------|----------------|
| Availability of ingest | A slow or failing beacon must never slow a customer's page |
| Integrity of stats | Spam can inflate numbers or flood breakdowns with junk values |
| Database size | The free tier is about 500 MB; unbounded cardinality fills it |
| Dashboard accounts | Account takeover exposes every site's traffic |
| Visitor privacy | No IPs, no cookies, no cross-day identity |

## How the ingest path behaves under attack

### Spam and floods

- **Per-IP token bucket**: 10 events/s, burst 50 (`INGEST_IP_RATE`,
  `INGEST_IP_BURST`). It is checked first, before JSON parsing or the site
  lookup, so a flood costs one map lookup per request. IPv6 clients are
  bucketed per /64, because a single host usually controls a whole /64 and
  could otherwise rotate addresses.
- **Per-site token bucket**: 200 events/s, burst 1,000 (`INGEST_SITE_RATE`,
  `INGEST_SITE_BURST`). This caps the damage when an attacker uses many IPs
  against one site, and stops one site from using up shared capacity.
- **Over the limit**: the response is `429` and the event is counted in
  `events_dropped_total{reason="rate_limited"}`. The real tracker ignores
  responses, so legitimate pages see no difference.
- **Bounded buffer**: if the limits still let too much through, the 10k
  ingest buffer drops events
  (`events_dropped_total{reason="buffer_full"}`) instead of slowing requests.
  Losing analytics is better than adding latency to the customer's page.
- **Bounded memory**: the limiters hold at most 100k keys. Fully refilled
  buckets are swept every minute. When the table is full of active keys, new
  keys are refused instead of growing the map.

### Spoofed client IPs

`X-Forwarded-For` can be written by the client, so the API only trusts the
hops its own proxies append (`TRUSTED_PROXY_HOPS`, default 0, which ignores
the header). Before Phase 11 the left-most entry was trusted. That let any
client choose its IP, which bypassed per-IP limits and minted unlimited
"unique visitors". `TestClientIP` covers the fix.

### Oversized and malformed payloads

- The beacon body is capped at 4 KB (`413` beyond that), and request headers
  at 16 KB. Header reads time out after 5 s, which limits slowloris-style
  connections.
- URL and referrer are capped at 2 KB before parsing. The stored path is
  capped at 200 bytes and rejected beyond that. The event name is limited to
  64 characters from `[A-Za-z0-9_.:-]`. Props are limited to 10 entries of
  100 bytes each.
- **Batch poisoning.** `url.Parse` decodes percent-escapes, so a path such
  as `/%00` or `/%ff` reaches the server as a NUL byte or invalid UTF-8.
  Postgres rejects both, and because ingest writes with `COPY`, one bad row
  would fail the whole 500-event batch it shares with honest events. Paths
  with control characters or invalid UTF-8 are therefore rejected with
  `400`. Referrers and `utm_source` values that fail the same check are
  dropped without rejecting the event. Browser and OS names come from the
  User-Agent, which the client controls, so they are sanitised and capped at
  32 bytes.
- `FuzzParse` and `FuzzNormalizeDomain` check these properties for any input:
  stored text is valid UTF-8 without control characters, lengths stay within
  their caps, and normalization is idempotent. The domain fuzzer found that
  `[::1]` normalized to a value that no longer parsed. Domains are now
  restricted to plain ASCII DNS names.

### Bad origins

- The `Origin` header must match the site's registered domain, or the request
  gets `403`. Requests with no `Origin` are rejected too, which drops the
  laziest scripts.
- **This is not a security boundary.** Any non-browser client can send any
  `Origin`. It stops other websites from embedding the tracker against your
  site, and stops casual misuse. It does not stop a determined spammer. The
  rate limits above are what bound that spammer.

### Bots

- User-Agents that the parser classifies as bots, and requests with no UA,
  are answered `202` and dropped (`events_dropped_total{reason="bot"}`).
  Answering `202` instead of an error avoids telling a bot it was detected.

### Cardinality attacks

Random paths, referrers or browser names could otherwise create unlimited
rows. Each layer bounds them:

- Raw events last 48 hours.
- Rollups keep the top 50 values per dimension per bucket and fold the rest
  into `(other)`.
- The per-site rate limit bounds how fast junk values can arrive at all.

## Dashboard and accounts

- **Passwords** are hashed with argon2id.
- **Login and registration** are limited to 10 attempts a minute per IP.
  This bounds online guessing to about 14,400 attempts a day per address.
- **Session cookies** are `HttpOnly` and `SameSite=Lax`, plus `Secure` when
  `COOKIE_SECURE=true`, which production requires. Only a hash of the
  session token is stored server-side.
- **CSRF.** State-changing routes use a double-submit token: a cookie value
  that must be echoed in `X-CSRF-Token`, compared in constant time.
  `SameSite=Lax` is a second layer.
- **Ownership.** Every `/api/sites/{id}` route checks that the site belongs
  to the session's user (`403` otherwise).
- **Response headers.** Every response sets `X-Content-Type-Options:
  nosniff`, `Referrer-Policy: no-referrer` and `X-Frame-Options: DENY`.

### Public dashboards

- A share link's slug is 100 random bits, and the slug is the only
  credential. The public API returns stats and the domain, never the site ID
  or owner.
- Turning sharing off deletes the slug. Turning it on again issues a new one,
  so a leaked link can be revoked.

### Operational endpoints

- `/metrics` requires `Authorization: Bearer $METRICS_TOKEN` and returns
  `404` when no token is set. The metrics reveal traffic volume and internal
  timings.
- `/internal/rollup` requires `Authorization: Bearer $ROLLUP_SECRET` and is
  hidden the same way. It is idempotent and guarded by an advisory lock, so
  even a leaked secret only lets someone trigger extra rollup work.

## Privacy

- IP and User-Agent are used in memory to compute
  `HMAC(daily_salt, site || ip || ua)`, a country and a browser, OS and
  device label. Neither is written anywhere.
- Salts rotate at 00:00 UTC and are deleted after 2 days. After that, no
  one, including the operator, can link a visitor ID back to an IP or across
  days.
- The rate limiters hold IP strings in memory for at most a few minutes,
  only to count requests. They are never logged or persisted, and request
  logs record no IPs.

## Known gaps and accepted risks

| Gap | Why accepted / what would fix it |
|-----|----------------------------------|
| A distributed spammer can stay under per-IP limits | Per-site limit bounds the damage; real fix needs proof-of-work or a CAPTCHA-free challenge, which conflicts with a 1 KB tracker |
| Limits, sessions and the site cache are per instance | Single-instance design (docs/decisions.md); scale-out moves them to Redis |
| No account lockout or email verification | Rate limit bounds guessing; lockout enables targeted denial of service |
| `PATCH /api/sites/{id}` lets owners choose a custom slug | Owner-only; a guessable custom slug is the owner's choice |
| Origin check is advisory | See "Bad origins" above |
