-- Schema inicial do portal de payback.

CREATE TABLE users (
    id            bigserial PRIMARY KEY,
    email         text NOT NULL UNIQUE CHECK (email = lower(email)),
    name          text NOT NULL DEFAULT '',
    -- Identificador estável do provedor de identidade (oid do Entra ID).
    subject       text UNIQUE,
    role          text NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now()
);

-- Política financeira versionada: cada alteração cria uma linha; a de maior id
-- é a vigente. Projetos fechados ficam presos à versão em que foram ativados.
CREATE TABLE settings_versions (
    id          bigserial PRIMARY KEY,
    payload     jsonb NOT NULL,
    created_by  bigint REFERENCES users (id),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE licensing_programs (
    key           text PRIMARY KEY,
    manufacturer  text NOT NULL,
    name          text NOT NULL,
    price_usd     numeric(12, 6) NOT NULL CHECK (price_usd > 0),
    revision      integer NOT NULL DEFAULT 0
);

CREATE TABLE licensing_models (
    id                   bigserial PRIMARY KEY,
    program_key          text NOT NULL REFERENCES licensing_programs (key),
    model                text NOT NULL,
    segment              text NOT NULL DEFAULT '',
    sort_order           integer NOT NULL DEFAULT 0,
    -- NULL enquanto o consumo não foi cadastrado: o modelo não pode ser usado.
    daily_points         numeric(12, 6) CHECK (daily_points > 0),
    service_description  text NOT NULL DEFAULT '',
    revision             integer NOT NULL DEFAULT 0,
    UNIQUE (program_key, model),
    CHECK (daily_points IS NULL OR service_description <> '')
);

CREATE TABLE projects (
    id                      bigserial PRIMARY KEY,
    owner_id                bigint NOT NULL REFERENCES users (id),
    client_name             text NOT NULL CHECK (client_name <> ''),
    name                    text NOT NULL CHECK (name <> ''),
    status                  text NOT NULL DEFAULT 'draft'
                            CHECK (status IN ('draft', 'active', 'completed', 'cancelled')),
    -- Projetos de teste ficam fora dos indicadores da carteira.
    is_test                 boolean NOT NULL DEFAULT false,
    duration_months         integer NOT NULL CHECK (duration_months BETWEEN 1 AND 120),
    monthly_revenue         numeric(14, 2) NOT NULL DEFAULT 0 CHECK (monthly_revenue >= 0),
    implementation_revenue  numeric(14, 2) NOT NULL DEFAULT 0 CHECK (implementation_revenue >= 0),
    expected_loss_rate      numeric(7, 6) NOT NULL DEFAULT 0 CHECK (expected_loss_rate BETWEEN 0 AND 1),
    monthly_support_cost    numeric(14, 2) NOT NULL DEFAULT 0 CHECK (monthly_support_cost >= 0),
    -- Cotação USD/BRL da proposta; aplicada a todos os itens em dólar.
    exchange_rate           numeric(10, 4) CHECK (exchange_rate > 0),
    dest_state              char(2) NOT NULL DEFAULT 'SP',
    contract_start_date     date,
    deal_id                 bigint,
    notes                   text NOT NULL DEFAULT '',
    -- Preenchido quando o projeto sai de rascunho: congela a política usada.
    settings_version_id     bigint REFERENCES settings_versions (id),
    -- Controle de concorrência otimista.
    version                 integer NOT NULL DEFAULT 1,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX projects_owner_idx ON projects (owner_id);

CREATE TABLE project_items (
    id            bigserial PRIMARY KEY,
    project_id    bigint NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    position      integer NOT NULL DEFAULT 0,
    name          text NOT NULL CHECK (name <> ''),
    kind          text NOT NULL CHECK (kind IN ('hardware', 'software', 'points_license')),
    quantity      integer NOT NULL CHECK (quantity BETWEEN 1 AND 10000),
    -- Preço na moeda original; a conversão para reais acontece no cálculo.
    unit_price    numeric(16, 4) NOT NULL DEFAULT 0 CHECK (unit_price >= 0),
    currency      text NOT NULL DEFAULT 'BRL' CHECK (currency IN ('BRL', 'USD')),
    origin_state  char(2),
    imported      boolean NOT NULL DEFAULT false,
    -- Premissas da licença por pontos, copiadas do catálogo na inclusão.
    lic_program_key          text,
    lic_model_id             bigint,
    lic_model_name           text,
    lic_daily_points         numeric(12, 6),
    lic_price_usd            numeric(12, 6),
    lic_service_description  text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (kind <> 'hardware' OR origin_state IS NOT NULL),
    CHECK (kind <> 'points_license' OR (lic_program_key IS NOT NULL AND lic_model_name IS NOT NULL
        AND lic_daily_points > 0 AND lic_price_usd > 0))
);
CREATE INDEX project_items_project_idx ON project_items (project_id, position, id);

-- Compartilhamento concede somente leitura.
CREATE TABLE project_shares (
    project_id  bigint NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id     bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id)
);

CREATE TABLE project_events (
    id           bigserial PRIMARY KEY,
    project_id   bigint NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id      bigint REFERENCES users (id),
    description  text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX project_events_project_idx ON project_events (project_id, id DESC);
