-- Passive model-distillation monitoring. Request features contain hashes and
-- numeric metadata only; raw prompts are never stored in these tables.

CREATE TABLE IF NOT EXISTS distillation_request_features (
    id               BIGSERIAL PRIMARY KEY,
    request_id       VARCHAR(64) NOT NULL,
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id       BIGINT NOT NULL DEFAULT 0,
    group_id         BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    model            VARCHAR(100) NOT NULL DEFAULT '',
    prompt_simhash   BIGINT NOT NULL DEFAULT 0,
    prompt_length    INT NOT NULL DEFAULT 0,
    message_count    INT NOT NULL DEFAULT 0,
    explicit_signal  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_distillation_request_features_nonnegative
        CHECK (user_id > 0 AND group_id > 0 AND api_key_id >= 0 AND prompt_length >= 0 AND message_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_distillation_features_user_group_created
    ON distillation_request_features (user_id, group_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_distillation_features_group_created
    ON distillation_request_features (group_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_distillation_features_created_brin
    ON distillation_request_features USING BRIN (created_at);

CREATE TABLE IF NOT EXISTS distillation_assessments (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id          BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    window_start      TIMESTAMPTZ NOT NULL,
    window_end        TIMESTAMPTZ NOT NULL,
    score             INT NOT NULL,
    evidence_classes  INT NOT NULL,
    mode              VARCHAR(16) NOT NULL,
    metrics           JSONB NOT NULL DEFAULT '{}'::jsonb,
    auto_disabled     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_distillation_assessment_window UNIQUE (user_id, group_id, window_start),
    CONSTRAINT chk_distillation_assessment_window CHECK (window_start < window_end),
    CONSTRAINT chk_distillation_assessment_score CHECK (score BETWEEN 0 AND 100),
    CONSTRAINT chk_distillation_assessment_evidence CHECK (evidence_classes BETWEEN 0 AND 5),
    CONSTRAINT chk_distillation_assessment_mode CHECK (mode IN ('observe', 'enforce')),
    CONSTRAINT chk_distillation_assessment_metrics CHECK (jsonb_typeof(metrics) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_distillation_assessments_user_group_window
    ON distillation_assessments (user_id, group_id, window_start DESC);
CREATE INDEX IF NOT EXISTS idx_distillation_assessments_score_created
    ON distillation_assessments (score DESC, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_distillation_assessments_created_brin
    ON distillation_assessments USING BRIN (created_at);
