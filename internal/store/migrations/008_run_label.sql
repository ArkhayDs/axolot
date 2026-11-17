-- The name results show and group a model by, computed once instead of in every query. Must match store.ModelLabel:
-- one cloud tag served by several API connections and by subscription counts as several models.
ALTER TABLE runs ADD COLUMN label text NOT NULL GENERATED ALWAYS AS (
    CASE connection_type
        WHEN 'api' THEN model || ' (API · ' || provider_name || ')'
        WHEN 'subscription' THEN model || ' (abo)'
        ELSE model
    END
) STORED;
