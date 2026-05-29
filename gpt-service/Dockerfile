FROM docker.m.daocloud.io/library/node:22-bookworm-slim AS dashboard_remote_builder

WORKDIR /gpt/webui
RUN sed -i 's/deb.debian.org/mirrors.ustc.edu.cn/g' /etc/apt/sources.list.d/debian.sources     && apt-get update     && apt-get install -y --no-install-recommends libprotobuf-dev protobuf-compiler     && rm -rf /var/lib/apt/lists/*
COPY common-lib/ui /common-lib/ui
COPY common-lib/proto /common-lib/proto
COPY common-lib/scripts /common-lib/scripts
COPY gpt/proto /gpt/proto
COPY gpt/webui ./
RUN npm ci && SOURCE_ROOT=/ npm run build

FROM docker.m.daocloud.io/library/golang:1.26-alpine AS gpt_account_builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV PATH=/root/go/bin:$PATH

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git protobuf-dev \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

COPY common-lib /common-lib
COPY gpt/gpt-account/go.mod gpt/gpt-account/go.sum ./
RUN go mod edit -replace github.com/byte-v-forge/common-lib=/common-lib \
    && go mod download

COPY gpt/proto ./proto
COPY gpt/gpt-account ./
RUN mkdir -p pb \
    && rm -f pb/*.pb.go pb/*_grpc.pb.go \
    && protoc -I proto --go_out=pb --go-grpc_out=pb proto/gpt_account.proto \
    && go build -o /out/gpt-account .

FROM docker.m.daocloud.io/library/golang:1.26-alpine AS orchestrator_builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV PATH=/root/go/bin:$PATH

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git protobuf-dev \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

COPY common-lib /common-lib
COPY gpt/pkg/gptplugin /gpt/pkg/gptplugin
COPY gpt/orchestrator/go.mod gpt/orchestrator/go.sum ./
RUN go mod edit -replace github.com/byte-v-forge/common-lib=/common-lib \
    && go mod edit -replace github.com/byte-v-forge/gpt/pkg/gptplugin=/gpt/pkg/gptplugin \
    && go mod download

COPY gpt/proto ./proto
COPY gpt/orchestrator ./
RUN go mod edit -replace github.com/byte-v-forge/common-lib=/common-lib \
    && go mod edit -replace github.com/byte-v-forge/gpt/pkg/gptplugin=/gpt/pkg/gptplugin \
    && mkdir -p pb \
    && rm -f pb/*.pb.go pb/*_grpc.pb.go \
    && protoc -I proto -I /common-lib/proto --go_out=pb --go-grpc_out=pb proto/*.proto \
    && go build -o /out/gpt-orchestrator .

FROM orchestrator_builder AS orchestrator_private_builder
ARG GPT_ORCHESTRATOR_BUILD_TAGS=private_plugins

COPY gpt-private/go.mod /gpt-private/go.mod
COPY gpt-private/plugins /gpt-private/plugins
RUN go mod edit -require github.com/byte-v-forge/gpt-private@v0.0.0 \
    && go mod edit -replace github.com/byte-v-forge/gpt-private=/gpt-private \
    && go mod download

RUN go mod edit -replace github.com/byte-v-forge/common-lib=/common-lib \
    && go mod edit -replace github.com/byte-v-forge/gpt/pkg/gptplugin=/gpt/pkg/gptplugin \
    && go mod edit -require github.com/byte-v-forge/gpt-private@v0.0.0 \
    && go mod edit -replace github.com/byte-v-forge/gpt-private=/gpt-private \
    && if [ -n "$GPT_ORCHESTRATOR_BUILD_TAGS" ]; then \
        go build -tags "$GPT_ORCHESTRATOR_BUILD_TAGS" -o /out/gpt-orchestrator .; \
      else \
        go build -o /out/gpt-orchestrator .; \
      fi

FROM docker.m.daocloud.io/library/golang:1.26-alpine AS gopay_app_builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV PATH=/root/go/bin:$PATH

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git protobuf-dev \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

COPY common-lib /common-lib
COPY gpt-private/gopay/go.mod gpt-private/gopay/go.sum ./
RUN go mod edit -replace github.com/byte-v-forge/common-lib=/common-lib \
    && go mod download

COPY gpt-private/gopay/proto ./proto
COPY gpt-private/gopay ./
RUN mkdir -p pb \
    && rm -f pb/*.pb.go pb/*_grpc.pb.go \
    && protoc -I proto --go_out=pb --go-grpc_out=pb proto/gopay_app.proto proto/payment.proto \
    && go build -o /out/gopay-app ./cmd/gopay-app-server \
    && go build -o /out/gopay-payment ./cmd/gopay-payment-server

FROM docker.m.daocloud.io/library/alpine:latest AS gpt_service_core_runtime

WORKDIR /app

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache bash ca-certificates nodejs

COPY --from=gpt_account_builder /out/gpt-account /app/bin/gpt-account
COPY --from=orchestrator_builder /out/gpt-orchestrator /app/bin/gpt-orchestrator
COPY --from=dashboard_remote_builder /gpt/webui/dist /app/dashboard/gpt
COPY gpt/gpt-service/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

EXPOSE 50051 50052 8080 8081
CMD ["/app/entrypoint.sh"]

FROM gpt_service_core_runtime AS gpt_service_private_runtime

COPY --from=orchestrator_private_builder /out/gpt-orchestrator /app/bin/gpt-orchestrator
COPY --from=gopay_app_builder /out/gopay-app /app/bin/gopay-app
COPY --from=gopay_app_builder /out/gopay-payment /app/bin/gopay-payment
COPY gpt-private/gopay-sidecar /app/gopay-sidecar
RUN chmod +x /app/gopay-sidecar/*.sh

EXPOSE 50054

FROM gpt_service_core_runtime AS gpt_service_runtime
