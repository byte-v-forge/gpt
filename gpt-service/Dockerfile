FROM docker.m.daocloud.io/library/golang:1.26-alpine AS account_builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV PATH=/root/go/bin:$PATH

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git protobuf-dev \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

COPY gpt/account-db/go.mod gpt/account-db/go.sum ./
RUN go mod download

COPY gpt/proto ./proto
COPY gpt/account-db ./
RUN mkdir -p pb \
    && rm -f pb/*.pb.go pb/*_grpc.pb.go \
    && protoc -I proto --go_out=pb --go-grpc_out=pb proto/account_db.proto \
    && go build -o /out/account-db .

FROM docker.m.daocloud.io/library/golang:1.26-alpine AS workflow_builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV PATH=/root/go/bin:$PATH

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git protobuf-dev \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

COPY browser-automation /browser-automation
COPY sms /sms
COPY gpt/orchestrator/go.mod gpt/orchestrator/go.sum ./
RUN go mod edit -replace github.com/byte-v-forge/browser-automation=/browser-automation \
    -replace github.com/byte-v-forge/sms=/sms \
    && go mod download

COPY gpt/proto ./proto
COPY mailbox/proto/email.proto ./proto/email.proto
COPY mailbox/proto/mailbox_service.proto ./proto/mailbox_service.proto
COPY gpt/orchestrator ./
RUN go mod edit -replace github.com/byte-v-forge/browser-automation=/browser-automation \
    -replace github.com/byte-v-forge/sms=/sms \
    && mkdir -p pb \
    && rm -f pb/*.pb.go pb/*_grpc.pb.go \
    && protoc -I proto --go_out=pb --go-grpc_out=pb proto/*.proto \
    && go build -o /out/gpt-workflows .

FROM docker.m.daocloud.io/library/golang:1.26-alpine AS gopay_app_builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV PATH=/root/go/bin:$PATH

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git protobuf-dev \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

COPY gpt/gopay/go.mod gpt/gopay/go.sum ./
RUN go mod download

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
    && apk add --no-cache bash ca-certificates

COPY --from=account_builder /out/account-db /app/bin/account-db
COPY --from=workflow_builder /out/gpt-workflows /app/bin/gpt-workflows
COPY --from=gopay_app_builder /out/gopay-app /app/bin/gopay-app
COPY --from=gopay_app_builder /out/gopay-payment /app/bin/gopay-payment

COPY gpt/gpt-service/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

EXPOSE 50051 50052 50054 8081
CMD ["/app/entrypoint.sh"]
