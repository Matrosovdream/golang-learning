-- Midigator Dashboard (hard) — full schema.
-- Faithful Go/Postgres port of the Laravel app's 22 domain tables.
-- Mounted into the postgres container's /docker-entrypoint-initdb.d on first boot.

-- ============================================================ identity & tenancy

CREATE TABLE IF NOT EXISTS tenants (
    id                          BIGSERIAL   PRIMARY KEY,
    name                        TEXT        NOT NULL,
    slug                        TEXT        NOT NULL UNIQUE,
    domain                      TEXT,
    midigator_api_secret        TEXT,
    midigator_sandbox_mode      BOOLEAN     NOT NULL DEFAULT FALSE,
    midigator_webhook_username  TEXT,
    midigator_webhook_password  TEXT,
    is_active                   BOOLEAN     NOT NULL DEFAULT TRUE,
    settings                    JSONB,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id                BIGSERIAL   PRIMARY KEY,
    tenant_id         BIGINT      REFERENCES tenants(id) ON DELETE SET NULL,
    email             TEXT        NOT NULL UNIQUE,
    password          TEXT        NOT NULL,
    name              TEXT        NOT NULL,
    avatar            TEXT,
    pin               TEXT,
    is_active         BOOLEAN     NOT NULL DEFAULT TRUE,
    is_platform_admin BOOLEAN     NOT NULL DEFAULT FALSE,
    email_verified_at TIMESTAMPTZ,
    last_login_at     TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);

CREATE TABLE IF NOT EXISTS site_settings (
    id          BIGSERIAL   PRIMARY KEY,
    tenant_id   BIGINT      REFERENCES tenants(id) ON DELETE CASCADE,
    key         TEXT        NOT NULL,
    value       TEXT,
    type        TEXT        NOT NULL DEFAULT 'string' CHECK (type IN ('string','integer','boolean','json')),
    "group"     TEXT        NOT NULL DEFAULT 'general',
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, key)
);

-- ============================================================ RBAC

CREATE TABLE IF NOT EXISTS rights (
    id          BIGSERIAL   PRIMARY KEY,
    name        TEXT        NOT NULL,
    slug        TEXT        NOT NULL UNIQUE,
    "group"     TEXT        NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS roles (
    id          BIGSERIAL   PRIMARY KEY,
    tenant_id   BIGINT      REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    slug        TEXT        NOT NULL,
    description TEXT,
    is_system   BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);

CREATE TABLE IF NOT EXISTS role_rights (
    id         BIGSERIAL   PRIMARY KEY,
    role_id    BIGINT      NOT NULL REFERENCES roles(id)  ON DELETE CASCADE,
    right_id   BIGINT      NOT NULL REFERENCES rights(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (role_id, right_id)
);

CREATE TABLE IF NOT EXISTS user_roles (
    id         BIGSERIAL   PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id    BIGINT      NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    granted_by BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, role_id)
);

CREATE TABLE IF NOT EXISTS manager_profiles (
    id            BIGSERIAL   PRIMARY KEY,
    user_id       BIGINT      NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    score         NUMERIC(5,2),
    notes         TEXT,
    assigned_mids JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================ case types

CREATE TABLE IF NOT EXISTS chargebacks (
    id                         BIGSERIAL   PRIMARY KEY,
    tenant_id                  BIGINT      NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    chargeback_guid            TEXT        NOT NULL UNIQUE,
    event_guid                 TEXT        NOT NULL,
    case_number                TEXT,
    arn                        TEXT,
    mid                        TEXT        NOT NULL,
    amount                     BIGINT      NOT NULL,
    currency                   TEXT        NOT NULL DEFAULT 'USD',
    card_brand                 TEXT,
    card_first_6               TEXT,
    card_last_4                TEXT,
    reason_code                TEXT,
    reason_description         TEXT,
    chargeback_date            DATE,
    date_received              DATE,
    due_date                   DATE,
    processor_transaction_id   TEXT,
    processor_transaction_date DATE,
    auth_code                  TEXT,
    alternate_transaction_id   TEXT,
    result                     TEXT        NOT NULL DEFAULT 'pending' CHECK (result IN ('pending','won','lost','pre_arbitration')),
    dnf_reason                 TEXT,
    dnf_timestamp              TIMESTAMPTZ,
    order_guid                 TEXT,
    order_id                   TEXT,
    crm_account_guid           TEXT,
    assigned_to                BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    stage                      TEXT        NOT NULL DEFAULT 'new' CHECK (stage IN ('new','in_review','action_taken','responded','resolved')),
    is_hidden                  BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_chargebacks_tenant_stage    ON chargebacks(tenant_id, stage);
CREATE INDEX IF NOT EXISTS idx_chargebacks_tenant_assigned ON chargebacks(tenant_id, assigned_to);
CREATE INDEX IF NOT EXISTS idx_chargebacks_tenant_mid      ON chargebacks(tenant_id, mid);
CREATE INDEX IF NOT EXISTS idx_chargebacks_due_date        ON chargebacks(due_date);

CREATE TABLE IF NOT EXISTS prevention_alerts (
    id                      BIGSERIAL   PRIMARY KEY,
    tenant_id               BIGINT      NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    prevention_guid         TEXT        NOT NULL UNIQUE,
    event_guid              TEXT        NOT NULL,
    prevention_case_number  TEXT,
    prevention_type         TEXT        NOT NULL CHECK (prevention_type IN ('ethoca','verifi','tc40')),
    arn                     TEXT,
    mid                     TEXT,
    amount                  BIGINT      NOT NULL,
    currency                TEXT        NOT NULL DEFAULT 'USD',
    card_first_6            TEXT,
    card_last_4             TEXT,
    merchant_descriptor     TEXT,
    prevention_timestamp    TIMESTAMPTZ,
    transaction_timestamp   TIMESTAMPTZ,
    resolution_type         TEXT,
    resolution_submitted_at TIMESTAMPTZ,
    order_guid              TEXT,
    order_id                TEXT,
    crm_account_guid        TEXT,
    assigned_to             BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    stage                   TEXT        NOT NULL DEFAULT 'new' CHECK (stage IN ('new','in_review','action_taken','resolved')),
    is_hidden               BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_preventions_tenant_stage    ON prevention_alerts(tenant_id, stage);
CREATE INDEX IF NOT EXISTS idx_preventions_tenant_assigned ON prevention_alerts(tenant_id, assigned_to);
CREATE INDEX IF NOT EXISTS idx_preventions_tenant_mid      ON prevention_alerts(tenant_id, mid);

CREATE TABLE IF NOT EXISTS orders (
    id                     BIGSERIAL   PRIMARY KEY,
    tenant_id              BIGINT      NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    order_guid             TEXT        NOT NULL UNIQUE,
    order_id               TEXT,
    mid                    TEXT,
    order_date             TIMESTAMPTZ,
    order_amount           BIGINT      NOT NULL,
    currency               TEXT        NOT NULL DEFAULT 'USD',
    card_brand             TEXT,
    card_first_6           TEXT,
    card_last_4            TEXT,
    card_exp_month         TEXT,
    card_exp_year          TEXT,
    avs                    TEXT,
    cvv                    TEXT,
    processor_auth_code    TEXT,
    processor_transaction_id TEXT,
    email                  TEXT,
    phone                  TEXT,
    ip_address             TEXT,
    billing_first_name     TEXT,
    billing_last_name      TEXT,
    billing_address        JSONB,
    refunded               BOOLEAN     NOT NULL DEFAULT FALSE,
    refunded_amount        BIGINT,
    subscription_cycle     INTEGER,
    subscription_parent_id TEXT,
    marketing_source       TEXT,
    sub_marketing_sources  JSONB,
    items                  JSONB,
    evidence               JSONB,
    is_hidden              BOOLEAN     NOT NULL DEFAULT FALSE,
    submission_status      TEXT        NOT NULL DEFAULT 'pending',
    submission_error       TEXT,
    submitted_at           TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_orders_tenant_mid        ON orders(tenant_id, mid);
CREATE INDEX IF NOT EXISTS idx_orders_tenant_order_id   ON orders(tenant_id, order_id);
CREATE INDEX IF NOT EXISTS idx_orders_tenant_submission ON orders(tenant_id, submission_status);

CREATE TABLE IF NOT EXISTS order_validations (
    id                        BIGSERIAL   PRIMARY KEY,
    tenant_id                 BIGINT      NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    order_validation_guid     TEXT        NOT NULL UNIQUE,
    event_guid                TEXT        NOT NULL,
    order_validation_type     TEXT,
    order_validation_timestamp TIMESTAMPTZ,
    transaction_timestamp     TIMESTAMPTZ,
    amount                    BIGINT      NOT NULL,
    currency                  TEXT        NOT NULL DEFAULT 'USD',
    card_first_6              TEXT,
    card_last_4               TEXT,
    arn                       TEXT,
    auth_code                 TEXT,
    card_brand                TEXT,
    order_guid                TEXT,
    order_id                  TEXT,
    crm_account_guid          TEXT,
    mid                       TEXT,
    assigned_to               BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    stage                     TEXT        NOT NULL DEFAULT 'new' CHECK (stage IN ('new','in_review','action_taken','resolved')),
    is_hidden                 BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_order_validations_tenant_stage ON order_validations(tenant_id, stage);
CREATE INDEX IF NOT EXISTS idx_order_validations_tenant_mid   ON order_validations(tenant_id, mid);

CREATE TABLE IF NOT EXISTS rdr_cases (
    id                  BIGSERIAL   PRIMARY KEY,
    tenant_id           BIGINT      NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rdr_guid            TEXT        NOT NULL UNIQUE,
    event_guid          TEXT        NOT NULL,
    rdr_case_number     TEXT,
    rdr_date            DATE,
    rdr_resolution      TEXT        CHECK (rdr_resolution IN ('accepted','declined')),
    arn                 TEXT,
    auth_code           TEXT,
    amount              BIGINT      NOT NULL,
    currency            TEXT        NOT NULL DEFAULT 'USD',
    card_first_6        TEXT,
    card_last_4         TEXT,
    merchant_descriptor TEXT,
    prevention_type     TEXT,
    transaction_date    DATE,
    order_guid          TEXT,
    order_id            TEXT,
    crm_account_guid    TEXT,
    assigned_to         BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    stage               TEXT        NOT NULL DEFAULT 'new' CHECK (stage IN ('new','in_review','resolved')),
    is_hidden           BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rdr_tenant_stage    ON rdr_cases(tenant_id, stage);
CREATE INDEX IF NOT EXISTS idx_rdr_tenant_assigned ON rdr_cases(tenant_id, assigned_to);

-- ============================================================ polymorphic / cross-cutting

CREATE TABLE IF NOT EXISTS comments (
    id               BIGSERIAL   PRIMARY KEY,
    commentable_type TEXT        NOT NULL,
    commentable_id   BIGINT      NOT NULL,
    user_id          BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body             TEXT        NOT NULL,
    is_internal      BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_comments_commentable ON comments(commentable_type, commentable_id);

CREATE TABLE IF NOT EXISTS evidence_files (
    id                BIGSERIAL   PRIMARY KEY,
    tenant_id         BIGINT      NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    evidenceable_type TEXT        NOT NULL,
    evidenceable_id   BIGINT      NOT NULL,
    uploaded_by       BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name              TEXT        NOT NULL,
    path              TEXT        NOT NULL,
    mime_type         TEXT,
    size_bytes        BIGINT      NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_evidence_tenant      ON evidence_files(tenant_id);
CREATE INDEX IF NOT EXISTS idx_evidence_evidenceable ON evidence_files(evidenceable_type, evidenceable_id);

CREATE TABLE IF NOT EXISTS stage_transitions (
    id             BIGSERIAL   PRIMARY KEY,
    trackable_type TEXT        NOT NULL,
    trackable_id   BIGINT      NOT NULL,
    user_id        BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    from_stage     TEXT        NOT NULL,
    to_stage       TEXT        NOT NULL,
    notes          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_stage_transitions_trackable ON stage_transitions(trackable_type, trackable_id);

CREATE TABLE IF NOT EXISTS activity_log (
    id            BIGSERIAL   PRIMARY KEY,
    tenant_id     BIGINT      NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id       BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    loggable_type TEXT        NOT NULL,
    loggable_id   BIGINT,
    action        TEXT        NOT NULL,
    metadata      JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_activity_loggable      ON activity_log(loggable_type, loggable_id);
CREATE INDEX IF NOT EXISTS idx_activity_tenant_created ON activity_log(tenant_id, created_at);

-- ============================================================ comms & ops

CREATE TABLE IF NOT EXISTS email_templates (
    id         BIGSERIAL   PRIMARY KEY,
    tenant_id  BIGINT      REFERENCES tenants(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL,
    subject    TEXT        NOT NULL,
    body       TEXT        NOT NULL,
    variables  JSONB,
    is_active  BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS email_logs (
    id              BIGSERIAL   PRIMARY KEY,
    tenant_id       BIGINT      NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emailable_type  TEXT        NOT NULL,
    emailable_id    BIGINT      NOT NULL,
    to_email        TEXT        NOT NULL,
    subject         TEXT        NOT NULL,
    body            TEXT        NOT NULL,
    template_id     BIGINT      REFERENCES email_templates(id) ON DELETE SET NULL,
    status          TEXT        NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','sent','failed','delivered','opened')),
    sent_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_email_logs_tenant_status ON email_logs(tenant_id, status);

CREATE TABLE IF NOT EXISTS webhook_logs (
    id            BIGSERIAL   PRIMARY KEY,
    tenant_id     BIGINT      NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    event_type    TEXT        NOT NULL,
    event_guid    TEXT        NOT NULL,
    payload       JSONB       NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'received' CHECK (status IN ('received','processed','failed')),
    error_message TEXT,
    processed_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_webhook_logs_tenant_type ON webhook_logs(tenant_id, event_type);
CREATE INDEX IF NOT EXISTS idx_webhook_logs_event_guid  ON webhook_logs(event_guid);

CREATE TABLE IF NOT EXISTS notifications (
    id              UUID        PRIMARY KEY,
    user_id         BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id       BIGINT      NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type            TEXT        NOT NULL,
    title           TEXT        NOT NULL,
    body            TEXT        NOT NULL,
    notifiable_type TEXT,
    notifiable_id   BIGINT,
    read_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_notifications_user_read   ON notifications(user_id, read_at);
CREATE INDEX IF NOT EXISTS idx_notifications_notifiable  ON notifications(notifiable_type, notifiable_id);

CREATE TABLE IF NOT EXISTS notification_settings (
    id              BIGSERIAL   PRIMARY KEY,
    user_id         BIGINT      NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    channel         TEXT        NOT NULL DEFAULT 'both' CHECK (channel IN ('email','in_app','both')),
    chargeback_new    BOOLEAN   NOT NULL DEFAULT TRUE,
    chargeback_result BOOLEAN   NOT NULL DEFAULT TRUE,
    prevention_new    BOOLEAN   NOT NULL DEFAULT TRUE,
    daily_digest      BOOLEAN   NOT NULL DEFAULT FALSE,
    weekly_report     BOOLEAN   NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
