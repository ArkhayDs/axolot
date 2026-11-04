-- Axolot schema: settings, model connections, RAG chunks, campaigns and their runs.
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE settings (
    key text PRIMARY KEY,
    value jsonb NOT NULL
);

CREATE TABLE providers (
    id bigserial PRIMARY KEY,
    name text NOT NULL UNIQUE,
    kind text NOT NULL CHECK (kind IN ('ollama', 'openai')),
    base_url text NOT NULL,
    api_key_enc bytea,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE case_models (
    case_id text NOT NULL,
    provider_id bigint NOT NULL REFERENCES providers (id) ON DELETE CASCADE,
    model text NOT NULL,
    PRIMARY KEY (case_id, provider_id, model)
);

-- Unconstrained vector: dimension depends on embed_model, and search always filters on it.
CREATE TABLE chunks (
    id bigserial PRIMARY KEY,
    corpus text NOT NULL,
    embed_model text NOT NULL,
    file text NOT NULL,
    position int NOT NULL,
    start_offset int NOT NULL,
    content text NOT NULL,
    embedding vector NOT NULL
);
CREATE INDEX chunks_corpus_model_idx ON chunks (corpus, embed_model);

CREATE TABLE campaigns (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    status text NOT NULL CHECK (status IN ('running', 'done', 'cancelled', 'interrupted')),
    hardware jsonb NOT NULL,
    rag_config jsonb NOT NULL,
    total_items int NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);

-- provider_name and category are copied so history survives a deleted connection or an edited case.
CREATE TABLE runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id uuid NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    case_id text NOT NULL,
    item_id text NOT NULL,
    category text NOT NULL,
    provider_name text NOT NULL,
    model text NOT NULL,
    output text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    ttft_ms bigint,
    total_ms bigint NOT NULL,
    load_ms bigint,
    prompt_tokens int,
    output_tokens int,
    tok_per_sec double precision,
    score double precision NOT NULL,
    checks jsonb NOT NULL,
    retrieved jsonb,
    retrieval_hit boolean,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX runs_campaign_idx ON runs (campaign_id, model, case_id);
