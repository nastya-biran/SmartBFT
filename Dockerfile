FROM golang:1.24 as builder

# Устанавливаем необходимые инструменты
RUN apt-get update && apt-get install -y \
    curl \
    unzip \
    protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

# Устанавливаем grpc-health-probe
RUN GRPC_HEALTH_PROBE_VERSION=v0.4.19 && \
    wget -qO/bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 && \
    chmod +x /bin/grpc_health_probe

WORKDIR /app

# Копируем файлы проекта
COPY . .

# Устанавливаем зависимости для генерации proto
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2

# Создаем директории для proto файлов
RUN mkdir -p examples/naive_chain/pkg/chain/proto

# Генерируем код из proto файлов
RUN protoc \
    --proto_path=. \
    --go_out=. \
    --go_opt=paths=source_relative \
    --go-grpc_out=. \
    --go-grpc_opt=paths=source_relative \
    examples/naive_chain/pkg/chain/proto/consensus.proto

# Устанавливаем зависимости
RUN go mod download

# Собираем приложение
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -o /usr/local/bin/smartbft-node ./examples/naive_chain/main.go

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/bin/smartbft-node /usr/local/bin/smartbft-node
COPY --from=builder /bin/grpc_health_probe /bin/grpc_health_probe

# Создаем директорию для данных
RUN mkdir -p /app/data && chmod 777 /app/data

EXPOSE 7050

HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=3 \
    CMD grpc_health_probe -addr=:7050 || exit 1

ENTRYPOINT ["smartbft-node"] 