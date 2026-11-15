import { FormEvent, useState } from "react";
import { api, DifalResult } from "../api";
import { money, parseNumber, pct, UFS } from "../format";
import { ErrorNote, Field, Row, useAction } from "../ui";

export function DifalPage() {
  const [base, setBase] = useState("");
  const [origin, setOrigin] = useState("ES");
  const [dest, setDest] = useState("SP");
  const [imported, setImported] = useState(false);
  const [result, setResult] = useState<DifalResult | null>(null);
  const calc = useAction();

  async function submit(e: FormEvent) {
    e.preventDefault();
    const value = parseNumber(base);
    if (value === null) return;
    const r = await calc.run(() => api.difal(value, origin, dest, imported));
    if (r) setResult(r);
  }
  const clear = () => setResult(null);

  return (
    <div className="page page-narrow">
      <header className="page-head">
        <h1>Calculadora de DIFAL</h1>
      </header>
      <p className="lead">
        Estima o diferencial de alíquota do ICMS na compra de equipamento de outro estado: alíquota interna do destino
        menos a interestadual, sobre o valor da compra.
      </p>
      <form onSubmit={submit} className="form">
        <div className="form-grid">
          <Field label="Valor do equipamento (R$)">
            <input className="input num" inputMode="decimal" required placeholder="0,00" value={base} onChange={(e) => { setBase(e.target.value); clear(); }} autoFocus />
          </Field>
          <Field label="UF do fornecedor">
            <select className="input" value={origin} onChange={(e) => { setOrigin(e.target.value); clear(); }}>
              {UFS.map((uf) => (
                <option key={uf}>{uf}</option>
              ))}
            </select>
          </Field>
          <Field label="UF de destino">
            <select className="input" value={dest} onChange={(e) => { setDest(e.target.value); clear(); }}>
              {UFS.map((uf) => (
                <option key={uf}>{uf}</option>
              ))}
            </select>
          </Field>
        </div>
        <label className="check">
          <input type="checkbox" checked={imported} onChange={(e) => { setImported(e.target.checked); clear(); }} />
          Mercadoria importada (alíquota interestadual de 4%)
        </label>
        <ErrorNote error={calc.error} />
        <div className="form-actions">
          <button type="submit" className="btn btn-primary" disabled={calc.busy}>
            Calcular DIFAL
          </button>
        </div>
      </form>
      {result && (
        <section className="block" role="status">
          <dl className="ledger">
            <Row label={`Alíquota interna de ${dest}`} value={pct(result.internalRate)} />
            <Row label="Alíquota interestadual" value={origin === dest ? "mesma UF" : pct(result.interstateRate)} />
            <Row label="DIFAL" value={pct(result.rate)} />
            <Row strong label="Valor do DIFAL" value={money(result.value)} />
          </dl>
        </section>
      )}
      <p className="form-note">
        É uma estimativa para a análise de viabilidade. Não considera base dupla, FCP, benefícios fiscais nem NCM.
        Confirme o valor com a contabilidade antes de fechar a compra.
      </p>
    </div>
  );
}
