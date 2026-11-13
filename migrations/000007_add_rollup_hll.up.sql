-- Serialized HyperLogLog of the bucket's visitor IDs (internal/hll). Daily
-- rows are built by merging these, so daily uniques no longer need raw
-- events. Rows written before this migration carry an empty sketch.
ALTER TABLE rollup_hourly ADD COLUMN visitors_hll BYTEA NOT NULL DEFAULT ''::bytea;
