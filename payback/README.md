# Informo Payback Portal

Nova versão do portal de análise de payback de locação da Informo, substituindo
a versão criada no Manus (`payback.informo.com.br`).

## Arquitetura

| Camada | Escolha |
|---|---|
| Backend | Go (binário único, com a interface embutida) |
| Motor financeiro | Go, pacote `internal/finance` |
| Interface | React + TypeScript |
| Banco | PostgreSQL |
| Autenticação | Microsoft Entra ID (Office 365), com camada isolada para Cloudflare Access no futuro |
| Entrega | Docker Compose em Linux próprio |

## Estado

- [x] Motor financeiro com testes (`internal/finance`)
- [x] Banco de dados (`internal/store`) e API (`internal/api`)
- [x] Login pelo Microsoft Entra ID (`internal/auth`)
- [x] Interface React (`web/`), embutida no binário
- [ ] Relatório PDF e Excel
- [ ] Empacotamento Docker

## Como rodar em desenvolvimento

```bash
export DATABASE_URL="postgres://usuario:senha@127.0.0.1:5432/payback"
export DEV_USER_EMAIL="voce@informo.com.br"   # entra sem login; só em localhost
export ADMIN_EMAILS="voce@informo.com.br"     # quem administra política e catálogo
go run ./cmd/server                           # escuta em 127.0.0.1:8080
```

O servidor cria as tabelas e carrega a política e o catálogo iniciais na
primeira execução. O modo `DEV_USER_EMAIL` entra sem login, só aceita escutar
em localhost e não pode ser combinado com as variáveis `ENTRA_*`: não existe
caminho para produção sem login. Todas as variáveis estão em `.env.example`.

## Interface

```bash
make build        # compila web/ e gera bin/payback com a interface embutida
cd web && npm run dev   # desenvolvimento, com proxy para o servidor em :8080
```

- **Telas**: carteira (por status, com busca e ordenação), novo projeto,
  projeto (decisão, gráfico de payback, condições, itens, mensalidade e
  simulador, memória de cálculo, fluxo mensal, histórico, compartilhamento),
  calculadora de DIFAL, catálogo de pontos, política financeira e auditoria.
- **Sem cálculo no navegador**: a interface só exibe o que a API devolve.
  Simulação e sensibilidade também são calculadas no servidor.
- **Fontes locais**: Archivo e IBM Plex Sans vão no pacote; a página não
  carrega nada de terceiros e a política de segurança de conteúdo proíbe.
- **Logotipo**: o nome "INFORMO" está em texto. Falta trocar pelo arquivo
  oficial da marca.

## Login

- **Fluxo**: OpenID Connect com código de autorização, PKCE, `state` e
  `nonce`, contra o Entra ID da Informo (locatário único). O token é validado
  por assinatura, emissor, audiência, validade e locatário.
- **Sessão**: fica no banco. O navegador recebe só um token aleatório em
  cookie `HttpOnly`, `Secure`, `SameSite=Lax` (prefixo `__Host-` em HTTPS), e
  o banco guarda o hash dele. Dura `SESSION_HOURS` (padrão 12 h).
- **Logout**: apaga a sessão no servidor; um cookie copiado deixa de valer.
- **Rotas**: `GET /auth/login?returnTo=/caminho`, `GET /auth/callback` e
  `POST /auth/logout`. A API responde 401 sem sessão e a interface redireciona
  para `/auth/login`.
- **Administradores**: os e-mails em `ADMIN_EMAILS` são promovidos ao entrar.

Os testes do login rodam contra um provedor OpenID Connect simulado. O login
contra o Entra real só pode ser verificado em uma máquina com acesso a
`login.microsoftonline.com` e com o segredo do cliente.

Testes de integração usam um PostgreSQL real, com um schema descartável por
teste:

```bash
TEST_DATABASE_URL="postgres://usuario:senha@127.0.0.1:5432/payback_test" go test ./...
```

## Banco e API

- **Nada de resultado gravado**: a análise é calculada a cada leitura a partir
  dos itens e da política, então não existe resultado defasado.
- **Política versionada**: cada alteração cria uma versão. Rascunhos usam a
  vigente; ao sair de rascunho o projeto congela a versão daquele momento.
- **Análise bloqueada**: quando falta premissa (câmbio, data de início, consumo
  de pontos) a API devolve `blocked` com as pendências, nunca números parciais.
- **Permissões**: o dono e o administrador editam; compartilhamento só lê;
  quem não tem acesso recebe 404. Política e catálogo são do administrador.
- **Auditoria**: toda alteração (projeto, item, compartilhamento, política,
  catálogo) grava quem fez, quando e se agiu como administrador em projeto
  alheio, na mesma transação da alteração. A trilha sobrevive à exclusão do
  projeto e o banco recusa alterá-la ou apagá-la.
- **Concorrência**: projeto, política e catálogo recusam gravação feita sobre
  uma versão antiga (HTTP 409).
- **Carteira**: `/api/dashboard` agrega por status e deixa de fora os projetos
  marcados como teste.

| Rota | Uso |
|---|---|
| `GET /api/me` | Usuário atual |
| `GET /api/dashboard` | Indicadores da carteira por status |
| `GET,POST /api/projects` | Lista com resumo; criação |
| `GET,PUT,DELETE /api/projects/{id}` | Projeto com itens e análise |
| `POST /api/projects/{id}/items` e `PUT,DELETE .../items/{itemId}` | Itens |
| `POST .../items/{itemId}/duplicate` | Duplicar item |
| `GET,POST /api/projects/{id}/shares` e `DELETE .../shares/{userId}` | Compartilhamento por e-mail |
| `GET /api/projects/{id}/events` | Histórico do projeto |
| `GET /api/projects/{id}/simulate?monthlyRevenue=` | Simular outra mensalidade, sem gravar |
| `GET /api/audit` | Trilha completa de auditoria (administrador) |
| `GET,PUT /api/settings` | Política financeira |
| `GET /api/catalog`, `PUT /api/catalog/programs/{key}`, `PUT /api/catalog/models/{id}` | Catálogo de pontos |
| `GET /api/difal` | Calculadora de DIFAL |
| `GET /healthz` | Verificação de saúde (inclui o banco) |

## Motor financeiro

Funções puras, sem banco nem estado global. Todo cálculo exibido em tela, PDF
ou Excel deve vir daqui.

```bash
go test ./...
```

| Arquivo | Conteúdo |
|---|---|
| `settings.go` | Política financeira e validação |
| `capex.go` | CAPEX a partir dos itens na moeda original |
| `difal.go` | Estimativa de DIFAL |
| `commission.go` | Comissão sobre a margem contratual |
| `engine.go` | Fluxo mensal, VPL, TIR, paybacks, mensalidades mínimas, cenários |
| `health.go` | Saúde do contrato e alerta de TIR |
| `licensing.go` | Custo mensal de licenças por pontos por dias corridos |

### Equivalência com o portal anterior

`equivalence_test.go` roda os exemplos gerados pelo motor anterior
(`testdata/exemplos_manus.json`, dados sintéticos) e exige os mesmos números,
mês a mês, em tudo que não foi alterado. `engine_test.go` também confere um
projeto real observado no portal anterior.

### O que mudou em relação ao portal anterior

1. **DIFAL**: a alíquota interestadual de 7% passou a valer só de Sul/Sudeste
   (exceto ES) para Norte, Nordeste, Centro-Oeste e ES. Para São Paulo a
   alíquota é 12% de qualquer origem, ou 4% para mercadoria importada. O
   destino deixou de ser fixo em SP e UF desconhecida gera erro.
2. **Payback**: a decisão usa o payback descontado sustentado, isto é, o mês a
   partir do qual o saldo não volta a ficar negativo. O primeiro cruzamento
   continua disponível em `FirstCrossingMonth`.
3. **Mensalidade mínima**: é encontrada rodando o próprio motor, em centavos,
   em vez de uma fórmula fechada que falhava quando a comissão era zero ou a
   implantação cobria o CAPEX. Sem solução, o resultado é "indisponível".
4. **Câmbio**: os itens guardam o preço na moeda original e a conversão
   acontece a cada cálculo, então mudar o câmbio reconverte CAPEX, DIFAL e
   licenças por pontos. A proposta tem uma única cotação.
5. **Entradas inválidas**: são recusadas com erro, não corrigidas em silêncio.
   O prazo precisa existir na política financeira.
6. **Unidades**: toda taxa é fração decimal, inclusive a TIR.

### Premissas mantidas, a confirmar

- Comissão paga integralmente no mês zero, com o DIFAL dentro do custo abatido.
- Impostos sobre o valor faturado, antes da inadimplência esperada.
- A carga "IBS+CBS" é uma alíquota única de simulação, não apuração fiscal.
- O DIFAL é a diferença simples de alíquotas; não considera base dupla, FCP ou
  benefícios. As alíquotas internas por UF vieram do portal anterior e precisam
  de conferência contábil.
- Licença por pontos é estimativa de consumo, sem compra, saldo ou reposição.
