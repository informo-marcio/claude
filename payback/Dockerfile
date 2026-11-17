# Imagem do portal de payback: um binário Go com a interface embutida, sem
# shell e sem root. Construa a partir da pasta payback/:
#   docker build -t informo/payback .

# 1. Interface React
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
# O Vite grava em ../internal/webui/dist, que o Go embute no binário.
RUN mkdir -p ../internal/webui/dist && npm run build

# 2. Servidor Go
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist/ ./internal/webui/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/payback ./cmd/server

# 3. Imagem final: só o binário e os certificados raiz (o login fala HTTPS
#    com a Microsoft). O fuso horário vai embutido no binário.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/payback /payback
USER 65532:65532
ENV LISTEN_ADDR=0.0.0.0:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/payback", "healthcheck"]
ENTRYPOINT ["/payback"]
