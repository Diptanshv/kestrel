CREATE TABLE events_raw (
  id           BIGSERIAL PRIMARY KEY,
  site_id      BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  ts           TIMESTAMPTZ NOT NULL,
  visitor_id   BYTEA NOT NULL CHECK (octet_length(visitor_id) = 16),  -- 16 bytes, valid within one UTC day only
  session_id   BIGINT NOT NULL,
  name         TEXT NOT NULL,                -- 'pageview' or custom event name
  pathname     TEXT NOT NULL,
  referrer     TEXT,                         -- host only, e.g. "news.ycombinator.com"
  utm_source   TEXT,
  country      CHAR(2),
  browser      TEXT,
  os           TEXT,
  device       TEXT                          -- 'desktop' | 'mobile' | 'tablet'
);

CREATE INDEX events_raw_site_id_ts_idx ON events_raw(site_id, ts);
