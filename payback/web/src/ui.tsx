import { ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { ApiError, Viability } from "./api";
import { viabilityLabel } from "./format";

/** Carrega dados da API e expõe estado de carga, erro e recarga. */
export function useLoad<T>(load: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(true);
  const seq = useRef(0);
  const reload = useCallback(() => {
    const current = ++seq.current;
    setLoading(true);
    load()
      .then((d) => {
        if (current !== seq.current) return;
        setData(d);
        setError(null);
      })
      .catch((e) => {
        if (current !== seq.current) return;
        setError(e instanceof ApiError ? e : new ApiError(0, String(e)));
      })
      .finally(() => {
        if (current === seq.current) setLoading(false);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  useEffect(reload, [reload]);
  return { data, setData, error, loading, reload };
}

/** Executa uma ação da API guardando erro e estado de envio. */
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);
  const run = useCallback(async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
    setBusy(true);
    setError(null);
    try {
      return await fn();
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError(0, String(e)));
      return undefined;
    } finally {
      setBusy(false);
    }
  }, []);
  return { busy, error, run, clear: () => setError(null) };
}

export function ErrorNote({ error, onRetry }: { error: ApiError | null; onRetry?: () => void }) {
  if (!error) return null;
  return (
    <div className="note note-error" role="alert">
      <p>{error.message}</p>
      {error.details.length > 0 && (
        <ul>
          {error.details.map((d) => (
            <li key={d}>{d}</li>
          ))}
        </ul>
      )}
      {onRetry && (
        <button type="button" className="btn btn-quiet" onClick={onRetry}>
          Tentar de novo
        </button>
      )}
    </div>
  );
}

export function Loading({ what }: { what: string }) {
  return (
    <p className="loading" role="status">
      Carregando {what}…
    </p>
  );
}

export function Field({
  label,
  hint,
  children,
  wide,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
  wide?: boolean;
}) {
  return (
    <label className={wide ? "field field-wide" : "field"}>
      <span className="field-label">{label}</span>
      {children}
      {hint && <span className="field-hint">{hint}</span>}
    </label>
  );
}

const viabilityClass: Record<Viability, string> = {
  VIAVEL: "tag tag-good",
  EXCECAO_COMERCIAL: "tag tag-warn",
  INVIAVEL: "tag tag-bad",
};

/** Decisão financeira: sempre com texto, nunca só cor. */
export function ViabilityTag({ value }: { value?: Viability }) {
  if (!value) return <span className="tag tag-muted">Pendente</span>;
  return <span className={viabilityClass[value]}>{viabilityLabel[value]}</span>;
}

/** Linha de rótulo e valor de uma memória de cálculo. */
export function Row({
  label,
  value,
  strong,
  sub,
}: {
  label: ReactNode;
  value: ReactNode;
  strong?: boolean;
  sub?: boolean;
}) {
  return (
    <div className={`ledger-row${strong ? " ledger-strong" : ""}${sub ? " ledger-sub" : ""}`}>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

/** Confirmação em dois passos para ações que não podem ser desfeitas. */
export function ConfirmButton({
  label,
  confirmLabel,
  onConfirm,
  disabled,
}: {
  label: string;
  confirmLabel: string;
  onConfirm: () => void;
  disabled?: boolean;
}) {
  const [armed, setArmed] = useState(false);
  if (!armed) {
    return (
      <button type="button" className="btn btn-quiet btn-danger" onClick={() => setArmed(true)} disabled={disabled}>
        {label}
      </button>
    );
  }
  return (
    <span className="confirm">
      <button
        type="button"
        className="btn btn-danger-solid"
        onClick={() => {
          setArmed(false);
          onConfirm();
        }}
      >
        {confirmLabel}
      </button>
      <button type="button" className="btn btn-quiet" onClick={() => setArmed(false)}>
        Cancelar
      </button>
    </span>
  );
}
