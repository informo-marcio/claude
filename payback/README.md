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
- [ ] Banco de dados e API
- [ ] Autenticação
- [ ] Interface
- [ ] Relatório PDF e Excel
- [ ] Empacotamento Docker

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
   acontece a cada cálculo, então mudar o câmbio reconverte CAPEX e DIFAL.
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
