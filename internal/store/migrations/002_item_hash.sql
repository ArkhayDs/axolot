-- Version of the question each answer replies to: cumulative stats only count the current versions.
-- NULL on older answers, which stay stored but out of the stats.
ALTER TABLE runs ADD COLUMN item_hash text;
