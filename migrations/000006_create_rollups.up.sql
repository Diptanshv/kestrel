-- One row per (site, bucket, dimension, value). dimension is '_total' (value
-- '') or one of page, referrer, event, country, browser, os, device.
-- Buckets start on the hour / day in the server's TIMEZONE so they line up
-- with the dashboard's own buckets.
CREATE TABLE rollup_hourly (
  site_id      BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  bucket       TIMESTAMPTZ NOT NULL,
  dimension    TEXT NOT NULL,
  value        TEXT NOT NULL,
  pageviews    INT NOT NULL,
  visitors     INT NOT NULL,       -- exact distinct visitors within the hour
  visits       INT NOT NULL,
  bounces      INT NOT NULL,
  duration_sum BIGINT NOT NULL,    -- seconds, summed over visits
  PRIMARY KEY (site_id, bucket, dimension, value)
);

CREATE TABLE rollup_daily (
  site_id      BIGINT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  day          DATE NOT NULL,
  dimension    TEXT NOT NULL,
  value        TEXT NOT NULL,
  pageviews    INT NOT NULL,
  visitors     INT NOT NULL,
  visits       INT NOT NULL,
  bounces      INT NOT NULL,
  duration_sum BIGINT NOT NULL,
  PRIMARY KEY (site_id, day, dimension, value)
);

-- Single row recording how far the job has got, so a run after downtime
-- resumes instead of starting over.
CREATE TABLE rollup_state (
  id             INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  hourly_through TIMESTAMPTZ NOT NULL,
  daily_through  DATE NOT NULL
);
