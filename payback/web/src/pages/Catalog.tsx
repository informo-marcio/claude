import { useState } from "react";
import { api, Model, Program } from "../api";
import { useUser } from "../App";
import { num, parseNumber, toField } from "../format";
import { ErrorNote, Loading, useAction, useLoad } from "../ui";

function ProgramPrice({ program, admin, onSaved }: { program: Program; admin: boolean; onSaved: () => void }) {
  const [text, setText] = useState<string | null>(null);
  const save = useAction();
  async function submit() {
    const price = parseNumber(text ?? "");
    if (price === null) return;
    const ok = await save.run(() => api.saveProgram(program.key, price, program.revision));
    if (ok) {
      setText(null);
      onSaved();
    }
  }
  return (
    <div className="program">
      <h2>
        {program.manufacturer} {program.name}
      </h2>
      {text === null ? (
        <p className="program-price">
          US$ {num(program.priceUsd, 2)} por ponto{" "}
          {admin && (
            <button type="button" className="btn-link" onClick={() => setText(toField(program.priceUsd, 6))}>
              Alterar preço
            </button>
          )}
        </p>
      ) : (
        <div className="inline-form">
          <input className="input num" inputMode="decimal" aria-label="Preço do ponto em dólar" value={text} onChange={(e) => setText(e.target.value)} autoFocus />
          <button type="button" className="btn btn-primary" disabled={save.busy} onClick={submit}>
            Salvar preço
          </button>
          <button type="button" className="btn btn-quiet" onClick={() => setText(null)}>
            Cancelar
          </button>
        </div>
      )}
      <ErrorNote error={save.error} />
    </div>
  );
}

function ModelRow({ model, admin, onSaved }: { model: Model; admin: boolean; onSaved: () => void }) {
  const [edit, setEdit] = useState<{ points: string; service: string } | null>(null);
  const save = useAction();
  async function submit() {
    if (!edit) return;
    const ok = await save.run(() => api.saveModel(model.id, parseNumber(edit.points), edit.service, model.revision));
    if (ok) {
      setEdit(null);
      onSaved();
    }
  }
  if (edit) {
    return (
      <tr>
        <td>
          <span className="row-client">{model.model}</span>
          <span className="row-name">{model.segment}</span>
        </td>
        <td>
          <input className="input" placeholder="Ex.: Licenciamento pleno" aria-label="Pacote de serviços" value={edit.service} onChange={(e) => setEdit({ ...edit, service: e.target.value })} />
        </td>
        <td>
          <input className="input num" inputMode="decimal" placeholder="em branco = pendente" aria-label="Pontos por dia" value={edit.points} onChange={(e) => setEdit({ ...edit, points: e.target.value })} autoFocus />
        </td>
        <td className="actions">
          <button type="button" className="btn-link" disabled={save.busy} onClick={submit}>
            Salvar
          </button>
          <button type="button" className="btn-link" onClick={() => setEdit(null)}>
            Cancelar
          </button>
          <ErrorNote error={save.error} />
        </td>
      </tr>
    );
  }
  return (
    <tr>
      <td>
        <span className="row-client">{model.model}</span>
        <span className="row-name">{model.segment}</span>
      </td>
      <td>{model.serviceDescription || <span className="muted">sem pacote cadastrado</span>}</td>
      <td className="num">
        {model.dailyPoints === null ? <span className="tag tag-warn">Pendente</span> : `${num(model.dailyPoints, 2)} por dia`}
      </td>
      {admin && (
        <td className="actions">
          <button type="button" className="btn-link" onClick={() => setEdit({ points: toField(model.dailyPoints, 6), service: model.serviceDescription })}>
            Editar
          </button>
        </td>
      )}
    </tr>
  );
}

export function CatalogPage() {
  const admin = useUser().role === "admin";
  const { data, error, reload } = useLoad(api.catalog, []);
  if (error) return <div className="page"><ErrorNote error={error} onRetry={reload} /></div>;
  if (!data) return <div className="page"><Loading what="o catálogo" /></div>;
  const pending = data.models.filter((m) => m.dailyPoints === null).length;
  return (
    <div className="page page-mid">
      <header className="page-head">
        <h1>Licenças por pontos</h1>
      </header>
      <p className="lead">
        Preço do ponto e consumo diário de cada modelo. Ao entrar em uma proposta, a licença copia esses valores e não
        muda mais, mesmo que o catálogo seja alterado depois.
      </p>
      {pending > 0 && (
        <p className="note">
          {pending} de {data.models.length} modelos ainda não têm consumo cadastrado e não podem entrar em propostas.
          {!admin && " Peça o cadastro a um administrador."}
        </p>
      )}
      {data.programs.map((p) => (
        <section key={p.key} className="block">
          <ProgramPrice program={p} admin={admin} onSaved={reload} />
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Modelo</th>
                  <th>Pacote de serviços</th>
                  <th className="num">Consumo de pontos</th>
                  {admin && <th aria-label="Ações" />}
                </tr>
              </thead>
              <tbody>
                {data.models
                  .filter((m) => m.programKey === p.key)
                  .map((m) => (
                    <ModelRow key={m.id} model={m} admin={admin} onSaved={reload} />
                  ))}
              </tbody>
            </table>
          </div>
        </section>
      ))}
    </div>
  );
}
