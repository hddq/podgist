FROM docker.io/library/golang:1.26-trixie AS builder

WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/podgist ./cmd/api

FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /app

COPY --from=builder /out/podgist /usr/local/bin/podgist
COPY --chown=nonroot:nonroot config.example.yaml /etc/podgist/config.yaml
COPY --chown=nonroot:nonroot migrations /app/migrations

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/podgist"]
