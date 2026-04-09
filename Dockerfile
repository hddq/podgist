ARG GO_VERSION
ARG NODE_VERSION=25

FROM docker.io/library/node:${NODE_VERSION}-trixie AS web-builder

WORKDIR /web

COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN npm install -g pnpm@10 && pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

FROM docker.io/library/golang:${GO_VERSION}-trixie AS builder

WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /web/build ./internal/webui/dist

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/podgist ./cmd/api

FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /app

COPY --from=builder /out/podgist /usr/local/bin/podgist
COPY --chown=nonroot:nonroot config.example.yaml /etc/podgist/config.yaml
COPY --chown=nonroot:nonroot migrations /app/migrations

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/podgist"]
