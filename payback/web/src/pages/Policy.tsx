import { FormEvent, useEffect, useState } from "react";
import { api, Policy } from "../api";
import { useUser } from "../App";
import { dateTime, parseNumber, toField } from "../format";
import { ErrorNote, Field, Loading, useAction, useLoad } from "../ui";

interface DurationRow {
  months: string;
  maxPayback: string;
  seller: string;
  architect: string;
  manager: string;
}

interface FormState {
  tax: string;
  opportunity: string;
  irr: string;
  loss: string;
  support: string;
  durations: DurationRow[];
  icms: Record<string, string>;
}

const p100 = (v: number) => toField(v * 100, 4);

function toForm(p: Policy): FormState {
  const s = p.settings;
  return {
    tax: p100(s.taxRate),
    opportunity: p100(s.monthlyOpportunityRate),
    irr: p100(s.irrAlertThreshold),
    loss: p100(s.defaultExpectedLossRate),
    support: toField(s.defaultMonthlySupportCost),
    durations: [...s.contractDurations]
      .sort((a, b) => a - b)
      .map((d) => {
        const c = s.commissionRatesByDuration[d] ?? { sellerRate: 0, architectRate: 0, managerRate: 0 };
        return {
          months: String(d),
          maxPayback: String(s.maxPaybackByDuration[d] ?? ""),
          seller: p100(c.sellerRate),
          architect: p100(c.architectRate),
          manager: p100(c.managerRate),
        };
      }),
    icms: Object.fromEntries(Object.entries(p.internalIcms).map(([uf, v]) => [uf, p100(v)])),
  };
}

const frac = (text: string) => (parseNumber(text) ?? 0) / 100;

function toPolicy(f: FormState): Policy {
  const durations = f.durations.map((d) => Number(d.months));
  return {
    settings: {
      taxRate: frac(f.tax),
      monthlyOpportunityRate: frac(f.opportunity),
      irrAlertThreshold: frac(f.irr),
      defaultExpectedLossRate: frac(f.loss),
      defaultMonthlySupportCost: parseNumber(f.support) ?? 0,
      contractDurations: durations,
      maxPaybackByDuration: Object.fromEntries(f.durations.map((d) => [d.months, Number(d.maxPayback)])),
      commissionRatesByDuration: Object.fromEntries(
        f.durations.map((d) => [
          d.months,
          { sellerRate: frac(d.seller), architectRate: frac(d.architect), managerRate: frac(d.manager) },
        ]),
      ),
    },
    internalIcms: Object.fromEntries(Object.entries(f.icms).map(([uf, v]) => [uf, frac(v)])),
  };
}

export function PolicyPage() {
  const user = useUser();
  const admin = user.role === "admin";
  const { data, setData, error, reload } = useLoad(api.settings, []);
  const [form, setForm] = useState<FormState | null>(null);
  const [saved, setSaved] = useState(false);
  const save = useAction();
  useEffect(() => {
    if (data) setForm(toForm(data.policy));
  }, [data]);

  if (error) return <div className="page"><ErrorNote error={error} onRetry={reload} /></div>;
  if (!data || !form) return <div className="page"><Loading what="a política financeira" /></div>;

  const set = (k: "tax" | "opportunity" | "irr" | "loss" | "support") => (e: { target: { value: string } }) => {
    setSaved(false);
    setForm((f) => f && { ...f, [k]: e.target.value });
  };
  const setDuration = (i: number, k: keyof DurationRow, v: string) => {
    setSaved(false);
    setForm((f) => f && { ...f, durations: f.durations.map((d, j) => (j === i ? { ...d, [k]: v } : d)) });
  };

  async function submit(e: FormEvent) {
    e.preventDefault();
    const result = await save.run(() => api.saveSettings(data!.id, toPolicy(form!)));
    if (result) {
      setData(result);
      setSaved(true);
    }
  }

  return (
    <div className="page page-mid">
      <header className="page-head">
        <h1>Política financeira</h1>
      </header>
      <p className="lead">
        Premissas usadas no cálculo de todas as propostas. Rascunhos passam a usar a política nova na hora; contratos já
        ativados continuam na política em que foram fechados.
      </p>
      <p className="meta">
        Versão {data.id}, gravada em {dateTime(data.createdAt)}.{" "}
        {!admin && "Somente administradores podem alterar."}
      </p>

      <form onSubmit={submit} className="form">
        <fieldset disabled={!admin || save.busy} className="fieldset">
          <h2>Premissas gerais</h2>
          <div className="form-grid">
            <Field label="Impostos sobre a receita (%)" hint="Carga estimada de IBS e CBS. É premissa de simulação, não apuração fiscal.">
              <input className="input num" inputMode="decimal" value={form.tax} onChange={set("tax")} />
            </Field>
            <Field label="Taxa de oportunidade (% ao mês)" hint="Quanto o caixa renderia parado. É a taxa de desconto do VPL.">
              <input className="input num" inputMode="decimal" value={form.opportunity} onChange={set("opportunity")} />
            </Field>
            <Field label="TIR mínima (% ao mês)" hint="Abaixo disso o projeto recebe alerta.">
              <input className="input num" inputMode="decimal" value={form.irr} onChange={set("irr")} />
            </Field>
            <Field label="Inadimplência padrão (%)" hint="Sugerida a projetos novos.">
              <input className="input num" inputMode="decimal" value={form.loss} onChange={set("loss")} />
            </Field>
            <Field label="Suporte padrão por mês (R$)" hint="Sugerido a projetos novos.">
              <input className="input num" inputMode="decimal" value={form.support} onChange={set("support")} />
            </Field>
          </div>

          <h2>Prazos, payback máximo e comissões</h2>
          <div className="table-wrap">
            <table className="table table-form">
              <thead>
                <tr>
                  <th>Prazo (meses)</th>
                  <th>Payback máximo (mês)</th>
                  <th>Vendedor (%)</th>
                  <th>Arquiteto (%)</th>
                  <th>Gerente (%)</th>
                  <th className="num">Comissão total</th>
                  {admin && <th aria-label="Ações" />}
                </tr>
              </thead>
              <tbody>
                {form.durations.map((d, i) => (
                  <tr key={i}>
                    <td>
                      <input className="input num" type="number" min={1} max={120} required aria-label="Prazo em meses" value={d.months} onChange={(e) => setDuration(i, "months", e.target.value)} />
                    </td>
                    <td>
                      <input className="input num" type="number" min={1} max={120} required aria-label={`Payback máximo para ${d.months} meses`} value={d.maxPayback} onChange={(e) => setDuration(i, "maxPayback", e.target.value)} />
                    </td>
                    {(["seller", "architect", "manager"] as const).map((k) => (
                      <td key={k}>
                        <input className="input num" inputMode="decimal" aria-label={`Comissão ${k} para ${d.months} meses`} value={d[k]} onChange={(e) => setDuration(i, k, e.target.value)} />
                      </td>
                    ))}
                    <td className="num">{toField((frac(d.seller) + frac(d.architect) + frac(d.manager)) * 100, 2)}%</td>
                    {admin && (
                      <td className="actions">
                        <button
                          type="button"
                          className="btn-link"
                          disabled={form.durations.length === 1}
                          onClick={() => setForm((f) => f && { ...f, durations: f.durations.filter((_, j) => j !== i) })}
                        >
                          Remover
                        </button>
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {admin && (
            <button
              type="button"
              className="btn btn-quiet"
              onClick={() =>
                setForm((f) => f && { ...f, durations: [...f.durations, { months: "", maxPayback: "", seller: "0", architect: "0", manager: "0" }] })
              }
            >
              Adicionar prazo
            </button>
          )}

          <details className="details">
            <summary>Alíquotas internas de ICMS por UF (usadas no DIFAL)</summary>
            <p className="block-text">Confira com a contabilidade: os estados alteram essas alíquotas com frequência.</p>
            <div className="icms-grid">
              {Object.keys(form.icms)
                .sort()
                .map((uf) => (
                  <label key={uf} className="icms-cell">
                    <span>{uf}</span>
                    <input
                      className="input num"
                      inputMode="decimal"
                      value={form.icms[uf]}
                      onChange={(e) => {
                        setSaved(false);
                        setForm((f) => f && { ...f, icms: { ...f.icms, [uf]: e.target.value } });
                      }}
                    />
                  </label>
                ))}
            </div>
          </details>
        </fieldset>

        <ErrorNote error={save.error} />
        {saved && (
          <p className="note note-ok" role="status">
            Política salva como versão {data.id}.
          </p>
        )}
        {admin && (
          <div className="form-actions">
            <button type="submit" className="btn btn-primary" disabled={save.busy}>
              {save.busy ? "Salvando…" : "Salvar política"}
            </button>
            <button type="button" className="btn btn-quiet" onClick={() => setForm(toForm(data.policy))}>
              Desfazer alterações
            </button>
          </div>
        )}
      </form>
    </div>
  );
}
