-- Trilha de auditoria: toda alteração feita no portal, por quem e quando.
-- Não tem chave estrangeira para projetos de propósito: o registro precisa
-- sobreviver à exclusão do projeto. Substitui project_events.

CREATE TABLE audit_log (
    id             bigserial PRIMARY KEY,
    created_at     timestamptz NOT NULL DEFAULT now(),
    user_id        bigint NOT NULL REFERENCES users (id),
    -- Cópias do momento da ação, para o registro não depender de cadastros.
    user_email     text NOT NULL,
    user_name      text NOT NULL DEFAULT '',
    -- Verdadeiro quando um administrador alterou projeto de outra pessoa.
    as_admin       boolean NOT NULL DEFAULT false,
    project_id     bigint,
    project_label  text,
    description    text NOT NULL
);
CREATE INDEX audit_log_project_idx ON audit_log (project_id, id DESC);

INSERT INTO audit_log (created_at, user_id, user_email, user_name, project_id, project_label, description)
SELECT e.created_at, u.id, u.email, u.name, p.id, p.client_name || ' — ' || p.name, e.description
FROM project_events e
JOIN projects p ON p.id = e.project_id
JOIN users u ON u.id = COALESCE(e.user_id, p.owner_id)
ORDER BY e.id;

DROP TABLE project_events;

-- A trilha só aceita inclusão: nem a própria aplicação consegue reescrevê-la.
CREATE FUNCTION audit_log_is_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log aceita somente inclusão';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_log_no_change
    BEFORE UPDATE OR DELETE OR TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_is_append_only();
