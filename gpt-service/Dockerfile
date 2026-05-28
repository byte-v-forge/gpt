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
COPY gpt/orchestrator/go.mod gpt/orchestrator/go.sum ./
RUN go mod edit -replace github.com/byte-v-forge/common-lib=/common-lib \
    && go mod download

COPY gpt/proto ./proto
COPY gpt/orchestrator ./
RUN go mod edit -replace github.com/byte-v-forge/common-lib=/common-lib \
    && mkdir -p pb \
    && rm -f pb/*.pb.go pb/*_grpc.pb.go \
    && protoc -I proto -I /common-lib/proto --go_out=pb --go-grpc_out=pb proto/*.proto \
    && go build -o /out/gpt-orchestrator .

FROM docker.m.daocloud.io/library/golang:1.26-alpine AS gopay_app_builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV PATH=/root/go/bin:$PATH

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git protobuf-dev \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

COPY common-lib /common-lib
COPY gpt/gopay/go.mod gpt/gopay/go.sum ./
RUN go mod edit -replace github.com/byte-v-forge/common-lib=/common-lib \
    && go mod download

COPY gpt/proto ./proto
COPY gpt/gopay ./
RUN mkdir -p pb \
    && rm -f pb/*.pb.go pb/*_grpc.pb.go \
    && protoc -I proto --go_out=pb --go-grpc_out=pb proto/gopay_app.proto proto/payment.proto \
    && go build -o /out/gopay-app ./cmd/gopay-app-server \
    && go build -o /out/gopay-payment ./cmd/gopay-payment-server

FROM docker.m.daocloud.io/library/alpine:latest

WORKDIR /app

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache bash ca-certificates nodejs

COPY --from=gpt_account_builder /out/gpt-account /app/bin/gpt-account
COPY --from=orchestrator_builder /out/gpt-orchestrator /app/bin/gpt-orchestrator
COPY --from=gopay_app_builder /out/gopay-app /app/bin/gopay-app
COPY --from=gopay_app_builder /out/gopay-payment /app/bin/gopay-payment
COPY --from=dashboard_remote_builder /gpt/webui/dist /app/dashboard/gpt
COPY gpt/gpt-service/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

EXPOSE 50051 50052 50054 8080 8081
CMD ["/app/entrypoint.sh"]
