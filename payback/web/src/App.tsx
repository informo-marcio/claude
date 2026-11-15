import { createContext, useContext } from "react";
import { NavLink, Route, Routes } from "react-router-dom";
import { api, User } from "./api";
import { ErrorNote, Loading, useLoad } from "./ui";
import { Portfolio } from "./pages/Portfolio";
import { ProjectNew } from "./pages/ProjectNew";
import { ProjectPage } from "./pages/Project";
import { CatalogPage } from "./pages/Catalog";
import { PolicyPage } from "./pages/Policy";
import { DifalPage } from "./pages/Difal";
import { AuditPage } from "./pages/Audit";

const UserContext = createContext<User | null>(null);

/** Usuário autenticado. Só é chamado dentro do App, depois da carga. */
export function useUser(): User {
  const u = useContext(UserContext);
  if (!u) throw new Error("usuário não carregado");
  return u;
}

async function signOut() {
  try {
    await api.logout();
  } catch {
    // No modo de desenvolvimento não existe sessão para encerrar.
  }
  window.location.assign("/auth/login");
}

export function App() {
  const me = useLoad(api.me, []);
  if (me.error) {
    return (
      <main className="boot">
        <ErrorNote error={me.error} onRetry={me.reload} />
      </main>
    );
  }
  if (!me.data) {
    return (
      <main className="boot">
        <Loading what="o portal" />
      </main>
    );
  }
  const user = me.data;
  return (
    <UserContext.Provider value={user}>
      <div className="shell">
        <aside className="rail">
          <NavLink to="/" className="brand" aria-label="Informo, payback de locação">
            <span className="brand-name">INFORMO</span>
            <span className="brand-sub">Payback de locação</span>
          </NavLink>
          <nav aria-label="Seções">
            <NavLink to="/" end>
              Carteira
            </NavLink>
            <NavLink to="/projetos/novo">Novo projeto</NavLink>
            <NavLink to="/difal">Calculadora de DIFAL</NavLink>
            <NavLink to="/catalogo">Licenças por pontos</NavLink>
            <NavLink to="/politica">Política financeira</NavLink>
            {user.role === "admin" && <NavLink to="/auditoria">Auditoria</NavLink>}
          </nav>
          <div className="rail-user">
            <span className="rail-user-name">{user.name || user.email}</span>
            <span className="rail-user-role">{user.role === "admin" ? "Administrador" : user.email}</span>
            <button type="button" className="rail-link" onClick={signOut}>
              Sair
            </button>
          </div>
        </aside>
        <main className="main">
          <Routes>
            <Route path="/" element={<Portfolio />} />
            <Route path="/projetos/novo" element={<ProjectNew />} />
            <Route path="/projetos/:id" element={<ProjectPage />} />
            <Route path="/difal" element={<DifalPage />} />
            <Route path="/catalogo" element={<CatalogPage />} />
            <Route path="/politica" element={<PolicyPage />} />
            <Route path="/auditoria" element={<AuditPage />} />
            <Route
              path="*"
              element={
                <div className="page">
                  <h1>Página não encontrada</h1>
                  <p>
                    O endereço não existe. Volte para a <NavLink to="/">carteira</NavLink>.
                  </p>
                </div>
              }
            />
          </Routes>
        </main>
      </div>
    </UserContext.Provider>
  );
}
