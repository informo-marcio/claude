import { FormEvent, useState } from "react";
import { Analysis, api, Catalog, Item, ItemInput, ItemKind, Project, ProjectDetail } from "../api";
import { kindLabel, money, num, parseNumber, toField, UFS } from "../format";
import { ConfirmButton, ErrorNote, Field, useAction } from "../ui";

interface FormState {
  kind: ItemKind;
  name: string;
  quantity: string;
  unitPrice: string;
  currency: "BRL" | "USD";
  originState: string;
  imported: boolean;
  modelId: string;
  refreshLicense: boolean;
}

const blank: FormState = {
  kind: "hardware",
  name: "",
  quantity: "1",
  unitPrice: "",
  currency: "BRL",
  originState: "SP",
  imported: false,
  modelId: "",
  refreshLicense: false,
};

function fromItem(it: Item): FormState {
  return {
    kind: it.kind,
    name: it.name,
    quantity: String(it.quantity),
    unitPrice: toField(it.unitPrice, 4),
    currency: it.currency,
    originState: it.originState ?? "SP",
    imported: it.imported,
    modelId: it.license ? String(it.license.modelId) : "",
    refreshLicense: false,
  };
}

export function Items({
  project,
  items,
  analysis,
  canEdit,
  onChanged,
}: {
  project: Project;
  items: Item[];
  analysis: Analysis;
  canEdit: boolean;
  onChanged: (d: ProjectDetail) => void;
}) {
  // editing: null = fechado, 0 = novo item, id = item em edição.
  const [editing, setEditing] = useState<number | null>(null);
  const [form, setForm] = useState<FormState>(blank);
  const [catalog, setCatalog] = useState<Catalog | null>(null);
  const action = useAction();

  async function ensureCatalog() {
    if (!catalog) {
      const c = await action.run(api.catalog);
      if (c) setCatalog(c);
    }
  }

  function open(item: Item | null) {
    action.clear();
    setForm(item ? fromItem(item) : blank);
    setEditing(item ? item.id : 0);
    if (item?.kind === "points_license") void ensureCatalog();
  }

  function setKind(kind: ItemKind) {
    setForm((f) => ({ ...f, kind }));
    if (kind === "points_license") void ensureCatalog();
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    const model = catalog?.models.find((m) => String(m.id) === form.modelId);
    const body: ItemInput =
      form.kind === "points_license"
        ? {
            kind: form.kind,
            name: form.name.trim() || (model ? model.model : ""),
            quantity: Number(form.quantity),
            modelId: Number(form.modelId),
            refreshLicense: form.refreshLicense,
          }
        : {
            kind: form.kind,
            name: form.name,
            quantity: Number(form.quantity),
            unitPrice: parseNumber(form.unitPrice) ?? 0,
            currency: form.currency,
            originState: form.kind === "hardware" ? form.originState : undefined,
            imported: form.kind === "hardware" ? form.imported : undefined,
          };
    const saved = await action.run(() =>
      editing ? api.updateItem(project.id, editing, body) : api.createItem(project.id, body),
    );
    if (saved) {
      onChanged(saved);
      setEditing(null);
    }
  }

  async function act(fn: () => Promise<ProjectDetail>) {
    const saved = await action.run(fn);
    if (saved) onChanged(saved);
  }

  // Os custos apurados seguem a ordem dos itens que entram no CAPEX.
  const costs = analysis.capex?.items ?? [];
  let costIndex = 0;
  const rows = items.map((it) => ({ it, cost: it.kind === "points_license" ? undefined : costs[costIndex++] }));
  const pendingModels = catalog?.models.filter((m) => m.dailyPoints === null).length ?? 0;

  return (
    <section className="block">
      <header className="block-head">
        <h2>Itens da proposta</h2>
        {canEdit && editing === null && (
          <button type="button" className="btn btn-quiet" onClick={() => open(null)}>
            Adicionar item
          </button>
        )}
      </header>

      {items.length === 0 && editing === null && (
        <p className="empty-line">
          Nenhum item ainda. {canEdit ? "Adicione os equipamentos e licenças para calcular o investimento." : ""}
        </p>
      )}

      {items.length > 0 && (
        <div className="table-wrap">
          <table className="table table-items">
            <thead>
              <tr>
                <th>Item</th>
                <th className="num">Qtd.</th>
                <th className="num">Preço unitário</th>
                <th className="num">DIFAL</th>
                <th className="num">Total</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(({ it, cost }) => (
                <tr key={it.id}>
                  <td>
                    <span className="row-client">{it.name}</span>
                    <span className="row-name">
                      {kindLabel[it.kind]}
                      {it.kind === "hardware" && `, origem ${it.originState}${it.imported ? ", importado" : ""}`}
                      {it.license && `, ${it.license.modelName}: ${num(it.license.dailyPoints, 2)} ponto(s) por dia a US$ ${num(it.license.priceUsd, 2)}`}
                    </span>
                    {canEdit && (
                      <span className="item-actions">
                        <button type="button" className="btn-link" onClick={() => open(it)}>
                          Editar
                        </button>
                        <button type="button" className="btn-link" onClick={() => act(() => api.duplicateItem(project.id, it.id))}>
                          Duplicar
                        </button>
                        <ConfirmButton label="Remover" confirmLabel="Remover item" onConfirm={() => act(() => api.deleteItem(project.id, it.id))} />
                      </span>
                    )}
                  </td>
                  <td className="num">{it.quantity}</td>
                  <td className="num">
                    {it.kind === "points_license"
                      ? "recorrente"
                      : it.currency === "USD"
                        ? `US$ ${num(it.unitPrice, 2)}`
                        : money(it.unitPrice)}
                  </td>
                  <td className="num">{cost?.difal && cost.difal.value > 0 ? money(cost.difal.value) : "–"}</td>
                  <td className="num">{cost ? money(cost.totalBrl) : it.kind === "points_license" ? "custo mensal" : "–"}</td>
                </tr>
              ))}
            </tbody>
            {analysis.capex && (
              <tfoot>
                <tr>
                  <td colSpan={3}>Investimento inicial (CAPEX)</td>
                  <td className="num">{money(analysis.capex.difal)}</td>
                  <td className="num">{money(analysis.capex.total)}</td>
                </tr>
                {analysis.licensing && analysis.licensing.totalCostBrl > 0 && (
                  <tr>
                    <td colSpan={4}>Licenças por pontos no contrato (custo recorrente)</td>
                    <td className="num">{money(analysis.licensing.totalCostBrl)}</td>
                  </tr>
                )}
              </tfoot>
            )}
          </table>
        </div>
      )}

      {editing === null && <ErrorNote error={action.error} />}

      {editing !== null && (
        <form onSubmit={submit} className="form form-inset">
          <h3>{editing ? "Editar item" : "Novo item"}</h3>
          <div className="form-grid">
            <Field label="Tipo">
              <select className="input" value={form.kind} onChange={(e) => setKind(e.target.value as ItemKind)} disabled={editing !== 0}>
                <option value="hardware">Hardware (com DIFAL)</option>
                <option value="software">Software ou licença paga à vista</option>
                <option value="points_license">Licença por pontos (custo mensal)</option>
              </select>
            </Field>

            {form.kind === "points_license" ? (
              <>
                <Field
                  label="Modelo do catálogo"
                  hint={pendingModels > 0 ? `${pendingModels} modelos ainda sem consumo cadastrado ficam indisponíveis.` : undefined}
                >
                  <select
                    className="input"
                    required
                    value={form.modelId}
                    onChange={(e) => setForm((f) => ({ ...f, modelId: e.target.value }))}
                  >
                    <option value="">{catalog ? "Escolha um modelo" : "Carregando catálogo…"}</option>
                    {catalog?.programs.map((p) => (
                      <optgroup key={p.key} label={`${p.manufacturer} ${p.name}`}>
                        {catalog.models
                          .filter((m) => m.programKey === p.key)
                          .map((m) => (
                            <option key={m.id} value={m.id} disabled={m.dailyPoints === null}>
                              {m.model}
                              {m.dailyPoints === null ? " (sem consumo cadastrado)" : `, ${m.serviceDescription}`}
                            </option>
                          ))}
                      </optgroup>
                    ))}
                  </select>
                </Field>
                <Field label="Descrição" hint="Em branco, usa o nome do modelo.">
                  <input className="input" maxLength={200} value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} />
                </Field>
              </>
            ) : (
              <>
                <Field label="Descrição">
                  <input className="input" required maxLength={200} value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} autoFocus />
                </Field>
                <Field label="Preço unitário">
                  <input className="input num" inputMode="decimal" required placeholder="0,00" value={form.unitPrice} onChange={(e) => setForm((f) => ({ ...f, unitPrice: e.target.value }))} />
                </Field>
                <Field label="Moeda" hint={form.currency === "USD" ? "Convertido pelo câmbio da proposta a cada cálculo." : undefined}>
                  <select className="input" value={form.currency} onChange={(e) => setForm((f) => ({ ...f, currency: e.target.value as "BRL" | "USD" }))}>
                    <option value="BRL">Real (R$)</option>
                    <option value="USD">Dólar (US$)</option>
                  </select>
                </Field>
              </>
            )}

            <Field label="Quantidade">
              <input className="input num" type="number" min={1} max={10000} required value={form.quantity} onChange={(e) => setForm((f) => ({ ...f, quantity: e.target.value }))} />
            </Field>

            {form.kind === "hardware" && (
              <Field label="UF do fornecedor" hint={`O DIFAL é calculado para entrega em ${project.destState}.`}>
                <select className="input" value={form.originState} onChange={(e) => setForm((f) => ({ ...f, originState: e.target.value }))}>
                  {UFS.map((uf) => (
                    <option key={uf}>{uf}</option>
                  ))}
                </select>
              </Field>
            )}
          </div>
          {form.kind === "hardware" && (
            <label className="check">
              <input type="checkbox" checked={form.imported} onChange={(e) => setForm((f) => ({ ...f, imported: e.target.checked }))} />
              Mercadoria importada (alíquota interestadual de 4%)
            </label>
          )}
          {form.kind === "points_license" && editing !== 0 && (
            <label className="check">
              <input type="checkbox" checked={form.refreshLicense} onChange={(e) => setForm((f) => ({ ...f, refreshLicense: e.target.checked }))} />
              Atualizar preço e consumo pelo catálogo atual
            </label>
          )}
          <ErrorNote error={action.error} />
          <div className="form-actions">
            <button type="submit" className="btn btn-primary" disabled={action.busy}>
              {action.busy ? "Salvando…" : editing ? "Salvar item" : "Adicionar item"}
            </button>
            <button type="button" className="btn btn-quiet" onClick={() => setEditing(null)}>
              Cancelar
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
