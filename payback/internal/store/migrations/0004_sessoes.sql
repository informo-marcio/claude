-- Sessões de login guardadas no servidor. O navegador recebe só um token
-- aleatório; aqui fica o hash dele, então vazar o banco não vaza sessões.
-- Apagar a linha encerra a sessão na hora (logout de verdade).

CREATE TABLE sessions (
    token_hash  bytea PRIMARY KEY,
    subject     text NOT NULL,
    email       text NOT NULL,
    name        text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL
);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);
