// Formatação em português do Brasil. Sempre vírgula decimal.

const brl = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" });
const brl0 = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL", maximumFractionDigits: 0 });

export const money = (v: number) => brl.format(v);
export const moneyRound = (v: number) => brl0.format(v);

export function compactMoney(v: number): string {
  const abs = Math.abs(v);
  const sign = v < 0 ? "−" : "";
  if (abs >= 1e6) return `${sign}R$ ${num(abs / 1e6, 2)} mi`;
  if (abs >= 1e4) return `${sign}R$ ${num(abs / 1e3, 0)} mil`;
  return sign + moneyRound(abs);
}

export function num(v: number, digits = 2): string {
  return new Intl.NumberFormat("pt-BR", { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(v);
}

/** Fração para percentual: 0.0378 vira "3,78%". */
export const pct = (fraction: number, digits = 2) => `${num(fraction * 100, digits)}%`;

export const pctMonth = (fraction: number | null) => (fraction === null ? "indisponível" : `${pct(fraction)} a.m.`);

export function date(iso: string): string {
  const [y, m, d] = iso.slice(0, 10).split("-");
  return `${d}/${m}/${y}`;
}

export function dateTime(iso: string): string {
  return new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" }).format(new Date(iso));
}

/** Lê um número digitado em pt-BR ("2.500,00"). Vazio ou inválido vira null. */
export function parseNumber(text: string): number | null {
  const clean = text.trim().replace(/\s|R\$|%/g, "");
  if (clean === "") return null;
  const normalized = clean.includes(",") ? clean.replace(/\./g, "").replace(",", ".") : clean;
  const v = Number(normalized);
  return Number.isFinite(v) ? v : null;
}

/** Número para o texto de um campo de edição. */
export function toField(v: number | null | undefined, digits = 2): string {
  if (v === null || v === undefined) return "";
  return new Intl.NumberFormat("pt-BR", { minimumFractionDigits: 0, maximumFractionDigits: digits, useGrouping: false }).format(v);
}

export const statusLabel: Record<string, string> = {
  draft: "Rascunho",
  active: "Ativo",
  completed: "Concluído",
  cancelled: "Cancelado",
};

export const viabilityLabel: Record<string, string> = {
  VIAVEL: "Viável",
  EXCECAO_COMERCIAL: "Exceção comercial",
  INVIAVEL: "Inviável",
};

export const healthLabel: Record<string, string> = {
  EXCELENTE: "Excelente",
  BOA: "Boa",
  ADEQUADA: "Adequada",
  LIMITE: "No limite",
  CRITICA: "Crítica",
};

export const kindLabel: Record<string, string> = {
  hardware: "Hardware",
  software: "Software",
  points_license: "Licença por pontos",
};

export const UFS = [
  "AC", "AL", "AM", "AP", "BA", "CE", "DF", "ES", "GO", "MA", "MG", "MS", "MT", "PA",
  "PB", "PE", "PI", "PR", "RJ", "RN", "RO", "RR", "RS", "SC", "SE", "SP", "TO",
];
