# Instalação do portal de payback

Guia para subir o portal em um servidor Linux da Informo com Docker, do zero
até o primeiro login. Todos os comandos rodam na pasta `payback/deploy/`.

## O que sobe

| Serviço | Função |
|---|---|
| `app` | O portal: API, interface e relatório em PDF, em um binário só. Roda sem root e com o sistema de arquivos somente leitura. |
| `db` | PostgreSQL 16. Sem porta exposta: só a aplicação e o backup o alcançam. |
| `backup` | Um `pg_dump` por dia, com retenção configurável. |
| `caddy` | Opcional. HTTPS automático com certificado do Let's Encrypt. |

## 1. Requisitos

- Linux com Docker Engine 24 ou mais novo e o plugin Compose v2
  (`docker compose version`).
- 1 vCPU, 1 GB de memória e 5 GB de disco são suficientes para começar.
- Saída HTTPS para `login.microsoftonline.com`. Sem ela o portal não sobe,
  porque não consegue validar o login.
- Para baixar as imagens: acesso ao Docker Hub.
- Com o Caddy: o DNS do domínio apontando para o servidor e as portas 80 e 443
  abertas para a internet (o Let's Encrypt valida por elas).

## 2. Código

```bash
git clone https://github.com/informo-marcio/claude.git
cd claude/payback/deploy
```

## 3. Configuração

```bash
cp .env.example .env
chmod 600 .env
openssl rand -hex 24      # use o resultado como POSTGRES_PASSWORD
```

Edite o `.env` e preencha:

- `POSTGRES_PASSWORD`: o valor gerado acima. Use só letras e números.
- `ENTRA_CLIENT_SECRET`: o valor do segredo criado no Entra, em
  **Registros de aplicativo > Informo Payback Portal > Certificados e
  segredos**. Ele aparece uma única vez, na criação.
- `DOMAIN`: já vem como `payback.informo.com.br`.
- `ADMIN_EMAILS`: quem administra política, catálogo e auditoria.

Os IDs do locatário e do aplicativo já vêm preenchidos.

Se usar outro domínio (um ambiente de homologação, por exemplo), cadastre
`https://<domínio>/auth/callback` como URI de redirecionamento no mesmo registro
do Entra, em **Authentication**. Sem isso a Microsoft recusa o login.

## 4. Subir

Escolha uma das duas formas de HTTPS.

### A. Com o Caddy (servidor sem proxy na frente)

```bash
docker compose --profile caddy up -d --build
```

O Caddy obtém e renova o certificado sozinho e redireciona HTTP para HTTPS.
`ACME_EMAIL` precisa estar preenchido no `.env`.

### B. Com um proxy reverso que já existe

```bash
docker compose up -d --build
```

A aplicação fica em `127.0.0.1:8080` (ou na porta de `APP_PORT`). Aponte o
proxy para ela repassando o cabeçalho `Host` e definindo
`X-Forwarded-Proto: https`. Exemplo para nginx:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto https;
}
```

Se o proxy estiver em outra máquina, troque em `compose.yaml` a porta
`127.0.0.1:${APP_PORT:-8080}:8080` pelo endereço da rede interna e libere o
acesso só para o proxy.

## 5. Conferir

```bash
docker compose ps                 # app e db devem aparecer como "healthy"
docker compose logs -f app        # deve mostrar "login pelo Entra ID habilitado" e "portal no ar"
```

Abra `https://payback.informo.com.br`, entre com a conta da Informo e confira:

1. Seu nome e "Administrador" aparecem no rodapé do menu.
2. A política financeira está em **Política financeira** (versão 1).
3. Crie um projeto de teste, marque como teste, adicione um item e baixe o PDF.

Se a organização não permitir que usuários consintam aplicativos, a primeira
tentativa mostra um pedido de aprovação. Um administrador do Microsoft 365
aprova em **Aplicativos empresariais > Informo Payback Portal > Permissões >
Conceder consentimento do administrador**.

## 6. Atualizar

```bash
git pull
docker compose exec backup /bin/sh /backup.sh now    # backup antes de atualizar
VERSION=$(git rev-parse --short HEAD) docker compose up -d --build   # acrescente --profile caddy se usar
```

As migrações do banco rodam sozinhas na partida. `VERSION` aparece no log
("portal no ar ... versao=") e mostra o que está no ar.

## 7. Backup

- Um backup por dia na hora `BACKUP_HOUR` (horário de Brasília), na pasta
  `BACKUP_DIR` (padrão `deploy/backups/`).
- Backups com mais de `BACKUP_RETENTION_DAYS` dias são apagados.
- Um backup que falha no meio não deixa arquivo; o log registra `FALHOU`.
- **Copie a pasta de backups para fora do servidor.** Backup no mesmo disco
  não protege contra perda do disco.

Backup na hora:

```bash
docker compose exec backup /bin/sh /backup.sh now
docker compose logs backup
```

## 8. Restaurar

Restaurar substitui todo o banco pelo conteúdo do backup.

```bash
docker compose stop app
docker compose exec db dropdb -U payback payback
docker compose exec db createdb -U payback payback
docker compose exec -T db pg_restore -U payback -d payback --no-owner --exit-on-error \
  < backups/payback-AAAAMMDD-HHMMSS.dump
docker compose start app
```

A trilha de auditoria volta inteira e continua protegida contra alteração. As
sessões abertas no momento do backup voltam junto; para obrigar todos a entrar
de novo, rode `docker compose exec db psql -U payback -c "DELETE FROM sessions"`.

## 9. Problemas comuns

| Sintoma | Causa provável |
|---|---|
| `app` reinicia e o log diz `descoberta do provedor` | O servidor não alcança `login.microsoftonline.com`. Se a saída passa por proxy corporativo, preencha `HTTPS_PROXY` no `.env`. Uma variável `HTTPS_PROXY` definida no shell tem precedência sobre o `.env`. |
| `app` reinicia e o log diz `defina ...` | Falta uma variável no `.env`. |
| A Microsoft responde que o endereço de resposta não confere | O domínio não está cadastrado como URI de redirecionamento no Entra (passo 3). |
| "Esta conta não pertence à organização" | A conta não é do locatário da Informo. |
| "Login expirado ou iniciado em outra aba" | O login demorou mais de 10 minutos ou foi iniciado em outra aba. Basta entrar de novo. |
| O Caddy não emite o certificado | DNS ainda não aponta para o servidor, ou as portas 80 e 443 estão fechadas. Veja `docker compose logs caddy`. |
| Página sem HTTPS (cadeado aberto) ou login em loop atrás de proxy próprio | O proxy não está enviando `X-Forwarded-Proto: https`, ou `DOMAIN` não confere com o endereço usado no navegador. |

## 10. Troca do portal antigo

O portal do Manus continua respondendo em `payback.informo.com.br` até o DNS
mudar. Um dia antes, reduza o TTL do registro para 5 minutos; depois aponte o
registro para o novo servidor. Os projetos antigos não são migrados.
