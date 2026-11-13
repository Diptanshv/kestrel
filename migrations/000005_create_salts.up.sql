-- One salt per UTC day. Rows older than two days are deleted, after which no
-- hash in events_raw can be linked back to an IP or across days by anyone,
-- including us.
CREATE TABLE salts (
  day  DATE PRIMARY KEY,
  salt BYTEA NOT NULL CHECK (octet_length(salt) = 32)
);
