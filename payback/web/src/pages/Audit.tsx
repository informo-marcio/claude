import { useState } from "react";
import { Link } from "react-router-dom";
import { api, AuditEvent } from "../api";
import { dateTime } from "../format";
import { ErrorNote, Loading, useAction, useLoad } from "../ui";

export function AuditPage() {
  const first = useLoad(() => api.audit(), []);
  const [older, setOlder] = useState<AuditEvent[]>([]);
  const [done, setDone] = useState(false);
  const more = useAction();

  if (first.error) return <div className="page"><ErrorNote error={first.error} onRetry={first.reload} /></div>;
  if (!first.data) return <div className="page"><Loading what="a trilha de auditoria" /></div>;
  const events = [...first.data, ...older];

  async function loadMore() {
    const last = events[events.length - 1];
    const next = await more.run(() => api.audit(last.id));
    if (next) {
      setOlder((o) => [...o, ...next]);
      if (next.length === 0) setDone(true);
    }
  }

  return (
    <div className="page">
      <header className="page-head">
        <h1>Auditoria</h1>
      </header>
      <p className="lead">
        Todas as alterações feitas no portal, incluindo projetos já excluídos, política financeira e catálogo. Os
        registros não podem ser editados nem apagados.
      </p>
      {events.length === 0 ? (
        <p className="empty-line">Nenhuma alteração registrada ainda.</p>
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>Quando</th>
                <th>Quem</th>
                <th>Projeto</th>
                <th>O que mudou</th>
              </tr>
            </thead>
            <tbody>
              {events.map((e) => (
                <tr key={e.id}>
                  <td className="nowrap">{dateTime(e.createdAt)}</td>
                  <td>
                    <span className="row-client">{e.userName || e.userEmail}</span>
                    <span className="row-name">
                      {e.userEmail}
                      {e.asAdmin ? ", como administrador" : ""}
                    </span>
                  </td>
                  <td>
                    {e.projectLabel === null ? (
                      <span className="muted">Configuração</span>
                    ) : e.projectId !== null && e.description !== "Projeto excluído" ? (
                      <Link to={`/projetos/${e.projectId}`}>{e.projectLabel}</Link>
                    ) : (
                      e.projectLabel
                    )}
                  </td>
                  <td>{e.description}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <ErrorNote error={more.error} />
      {events.length >= 200 && !done && (
        <button type="button" className="btn btn-quiet" disabled={more.busy} onClick={loadMore}>
          {more.busy ? "Carregando…" : "Carregar registros mais antigos"}
        </button>
      )}
    </div>
  );
}
