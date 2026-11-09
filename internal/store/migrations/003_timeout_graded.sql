-- A model that misses the 5 minute limit now fails the question with a score of 0 instead of staying unmeasured.
-- Earlier timeouts were stored as errors: they take the form the runner now writes.
UPDATE runs SET error = '', score = 0, total_ms = 300000, checks = '[{"name": "timeout", "passed": false}]'
WHERE error = 'no answer within 5m0s';
