-- Claude CLI time reported by the bridge, inside ttft_ms and total_ms: results can show the model's own time.
-- NULL outside the bridge and on older answers.
ALTER TABLE runs ADD COLUMN cli_startup_ms bigint, ADD COLUMN cli_exit_ms bigint;
