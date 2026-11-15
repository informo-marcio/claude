// Cliente da API do portal. Os tipos espelham o JSON do servidor; nenhum
// cálculo financeiro é feito no navegador.

export type Viability = "VIAVEL" | "EXCECAO_COMERCIAL" | "INVIAVEL";
export type HealthStatus = "EXCELENTE" | "BOA" | "ADEQUADA" | "LIMITE" | "CRITICA";
export type IRRStatus = "ACIMA_DO_LIMITE" | "ABAIXO_DO_LIMITE" | "NAO_CALCULAVEL";
export type ProjectStatus = "draft" | "active" | "completed" | "cancelled";
export type Access = "owner" | "shared" | "admin";
export type ItemKind = "hardware" | "software" | "points_license";

export interface User {
  id: number;
  email: string;
  name: string;
  role: "user" | "admin";
}

export interface Project {
  id: number;
  ownerId: number;
  ownerName: string;
  clientName: string;
  name: string;
  status: ProjectStatus;
  isTest: boolean;
  durationMonths: number;
  monthlyRevenue: number;
  implementationRevenue: number;
  expectedLossRate: number;
  monthlySupportCost: number;
  exchangeRate: number | null;
  destState: string;
  contractStartDate: string | null;
  dealId: number | null;
  notes: string;
  settingsVersionId: number | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface License {
  programKey: string;
  modelId: number;
  modelName: string;
  dailyPoints: number;
  priceUsd: number;
  serviceDescription: string;
}

export interface Item {
  id: number;
  position: number;
  name: string;
  kind: ItemKind;
  quantity: number;
  unitPrice: number;
  currency: "BRL" | "USD";
  originState: string | null;
  imported: boolean;
  license: License | null;
}

export interface Commission {
  rates: { sellerRate: number; architectRate: number; managerRate: number };
  contractRevenue: number;
  capexDeduction: number;
  licensingDeduction: number;
  grossBase: number;
  taxes: number;
  netBase: number;
  seller: number;
  architect: number;
  manager: number;
  total: number;
}

export interface MonthRow {
  month: number;
  grossRevenue: number;
  expectedLoss: number;
  taxes: number;
  supportCost: number;
  licensingCost: number;
  netCashFlow: number;
  discountedFlow: number;
  cumulativeNominal: number;
  cumulativeDiscounted: number;
}

export interface Scenario {
  capex: number;
  initialFlow: number;
  totalRevenue: number;
  taxes: number;
  revenueAfterTaxes: number;
  implementationRevenue: number;
  implementationNet: number;
  totalExpectedLoss: number;
  totalSupportCost: number;
  totalLicensingCost: number;
  commission: Commission;
  netCashAfterRisks: number;
  netCashResult: number;
  cashAfterPayback: number;
  npv: number;
  roiOnCapex: number;
  npvMargin: number;
  monthlyIrr: number | null;
  simplePaybackMonth: number | null;
  firstCrossingMonth: number | null;
  paybackMonth: number | null;
  maxPaybackMonth: number;
  minimumEconomicMonthly: number | null;
  minimumPolicyMonthly: number | null;
  meetsPaybackPolicy: boolean;
  viability: Viability;
  months: MonthRow[];
}

export interface ItemCost {
  name: string;
  kind: ItemKind;
  unitPriceBrl: number;
  baseBrl: number;
  difal?: { interstateRate: number; internalRate: number; rate: number; value: number };
  totalBrl: number;
}

export interface SensitivityRow {
  delta: number;
  monthlyRevenue: number;
  totalRevenue: number;
  npv: number;
  paybackMonth: number | null;
  monthlyIrr: number | null;
  viability: Viability;
}

export interface Analysis {
  status: "ready" | "blocked";
  blockers?: string[];
  settingsVersionId: number;
  capex?: { hardware: number; software: number; difal: number; total: number; items: ItemCost[] };
  licensing?: { totalCostBrl: number; monthlyCosts: number[] };
  scenarios?: { base: Scenario; pessimistic: Scenario };
  health?: { status: HealthStatus; score: number; bufferMonths: number; hasPayback: boolean };
  irr?: { status: IRRStatus; threshold: number; difference: number | null };
  sensitivity?: SensitivityRow[];
}

export interface ProjectDetail {
  project: Project;
  access: Access;
  items: Item[];
  analysis: Analysis;
}

export interface Summary {
  status: "ready" | "blocked";
  capex: number;
  totalRevenue: number;
  npv: number;
  paybackMonth: number | null;
  monthlyIrr: number | null;
  viability?: Viability;
  health?: HealthStatus;
  irrStatus?: IRRStatus;
}

export interface ProjectRow {
  project: Project;
  access: Access;
  summary: Summary;
}

export interface CommissionRates {
  sellerRate: number;
  architectRate: number;
  managerRate: number;
}

export interface Policy {
  settings: {
    taxRate: number;
    monthlyOpportunityRate: number;
    irrAlertThreshold: number;
    contractDurations: number[];
    maxPaybackByDuration: Record<string, number>;
    commissionRatesByDuration: Record<string, CommissionRates>;
    defaultExpectedLossRate: number;
    defaultMonthlySupportCost: number;
  };
  internalIcms: Record<string, number>;
}

export interface SettingsVersion {
  id: number;
  policy: Policy;
  createdAt: string;
}

export interface Program {
  key: string;
  manufacturer: string;
  name: string;
  priceUsd: number;
  revision: number;
}

export interface Model {
  id: number;
  programKey: string;
  model: string;
  segment: string;
  dailyPoints: number | null;
  serviceDescription: string;
  revision: number;
}

export interface Catalog {
  programs: Program[];
  models: Model[];
}

export interface AuditEvent {
  id: number;
  userName: string;
  userEmail: string;
  asAdmin: boolean;
  projectId: number | null;
  projectLabel: string | null;
  description: string;
  createdAt: string;
}

export interface Share {
  userId: number;
  email: string;
  name: string;
}

export interface DifalResult {
  interstateRate: number;
  internalRate: number;
  rate: number;
  value: number;
}

export class ApiError extends Error {
  status: number;
  details: string[];
  constructor(status: number, message: string, details: string[] = []) {
    super(message);
    this.status = status;
    this.details = details;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      method,
      credentials: "same-origin",
      headers: method === "GET" ? undefined : { "Content-Type": "application/json" },
      body: method === "GET" ? undefined : JSON.stringify(body ?? {}),
    });
  } catch {
    throw new ApiError(0, "Sem conexão com o servidor. Verifique a rede e tente de novo.");
  }
  if (res.status === 401) {
    // Sessão vencida: volta para o login e retorna a esta página depois.
    const here = window.location.pathname + window.location.search;
    window.location.assign("/auth/login?returnTo=" + encodeURIComponent(here));
    throw new ApiError(401, "Sessão expirada.");
  }
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => null);
  if (!res.ok) {
    throw new ApiError(res.status, data?.error ?? "Erro inesperado.", data?.details ?? []);
  }
  return data as T;
}

export interface ProjectInput {
  clientName: string;
  name: string;
  status?: ProjectStatus;
  isTest: boolean;
  durationMonths: number;
  monthlyRevenue: number;
  implementationRevenue: number;
  expectedLossRate?: number;
  monthlySupportCost?: number;
  exchangeRate: number | null;
  destState: string;
  contractStartDate: string | null;
  dealId: number | null;
  notes: string;
  version?: number;
}

export interface ItemInput {
  name: string;
  kind: ItemKind;
  quantity: number;
  unitPrice?: number;
  currency?: "BRL" | "USD";
  originState?: string;
  imported?: boolean;
  modelId?: number;
  refreshLicense?: boolean;
}

export const api = {
  me: () => request<User>("GET", "/api/me"),
  projects: () => request<ProjectRow[]>("GET", "/api/projects"),
  project: (id: number) => request<ProjectDetail>("GET", `/api/projects/${id}`),
  createProject: (body: ProjectInput) => request<ProjectDetail>("POST", "/api/projects", body),
  updateProject: (id: number, body: ProjectInput) => request<ProjectDetail>("PUT", `/api/projects/${id}`, body),
  deleteProject: (id: number) => request<void>("DELETE", `/api/projects/${id}`),
  simulate: (id: number, monthly: number) =>
    request<Analysis>("GET", `/api/projects/${id}/simulate?monthlyRevenue=${monthly}`),
  events: (id: number) => request<AuditEvent[]>("GET", `/api/projects/${id}/events`),
  createItem: (id: number, body: ItemInput) => request<ProjectDetail>("POST", `/api/projects/${id}/items`, body),
  updateItem: (id: number, itemId: number, body: ItemInput) =>
    request<ProjectDetail>("PUT", `/api/projects/${id}/items/${itemId}`, body),
  deleteItem: (id: number, itemId: number) => request<ProjectDetail>("DELETE", `/api/projects/${id}/items/${itemId}`),
  duplicateItem: (id: number, itemId: number) =>
    request<ProjectDetail>("POST", `/api/projects/${id}/items/${itemId}/duplicate`),
  shares: (id: number) => request<Share[]>("GET", `/api/projects/${id}/shares`),
  addShare: (id: number, email: string) => request<Share[]>("POST", `/api/projects/${id}/shares`, { email }),
  removeShare: (id: number, userId: number) => request<void>("DELETE", `/api/projects/${id}/shares/${userId}`),
  settings: () => request<SettingsVersion>("GET", "/api/settings"),
  saveSettings: (baseId: number, policy: Policy) =>
    request<SettingsVersion>("PUT", "/api/settings", { baseId, policy }),
  catalog: () => request<Catalog>("GET", "/api/catalog"),
  saveProgram: (key: string, priceUsd: number, revision: number) =>
    request<Program>("PUT", `/api/catalog/programs/${key}`, { priceUsd, revision }),
  saveModel: (id: number, dailyPoints: number | null, serviceDescription: string, revision: number) =>
    request<Model>("PUT", `/api/catalog/models/${id}`, { dailyPoints, serviceDescription, revision }),
  difal: (base: number, origin: string, dest: string, imported: boolean) =>
    request<DifalResult>("GET", `/api/difal?base=${base}&origin=${origin}&dest=${dest}&imported=${imported}`),
  audit: (before = 0) => request<AuditEvent[]>("GET", `/api/audit?before=${before}`),
  logout: () => request<void>("POST", "/auth/logout"),
};
