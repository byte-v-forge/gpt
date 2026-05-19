FROM docker.m.daocloud.io/library/golang:1.26-alpine AS account_builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV PATH=/root/go/bin:$PATH

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git protobuf-dev \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

COPY account-db/go.mod account-db/go.sum ./
RUN go mod download

COPY proto ./proto
COPY account-db ./
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

COPY orchestrator/go.mod orchestrator/go.sum ./
RUN go mod download

COPY proto ./proto
COPY orchestrator ./
RUN mkdir -p pb \
    && rm -f pb/*.pb.go pb/*_grpc.pb.go \
    && protoc -I proto --go_out=pb --go-grpc_out=pb proto/*.proto \
    && go build -o /out/gpt-workflows .

FROM docker.m.daocloud.io/library/python:3.12-slim

WORKDIR /app

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV DEBIAN_FRONTEND=noninteractive
ENV PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple
ENV PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn

RUN if [ -f /etc/apt/sources.list.d/debian.sources ]; then \
      sed -i \
        -e 's|https\?://deb.debian.org/debian|https://mirrors.tuna.tsinghua.edu.cn/debian|g' \
        -e 's|https\?://security.debian.org/debian-security|https://mirrors.tuna.tsinghua.edu.cn/debian-security|g' \
        /etc/apt/sources.list.d/debian.sources; \
    elif [ -f /etc/apt/sources.list ]; then \
      sed -i \
        -e 's|https\?://deb.debian.org/debian|https://mirrors.tuna.tsinghua.edu.cn/debian|g' \
        -e 's|https\?://security.debian.org/debian-security|https://mirrors.tuna.tsinghua.edu.cn/debian-security|g' \
        /etc/apt/sources.list; \
    fi \
    && apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY channels/gopay/payment/gopay-flow/requirements.txt /tmp/payment-requirements.txt
RUN pip install --no-cache-dir -r /tmp/payment-requirements.txt \
    && pip install --no-cache-dir grpcio grpcio-tools "protobuf>=6.31.1" "requests[socks]" "psycopg[binary]"

COPY --from=account_builder /out/account-db /app/bin/account-db
COPY --from=workflow_builder /out/gpt-workflows /app/bin/gpt-workflows

COPY proto/gopay_app.proto /app/proto/gopay_app.proto
COPY proto/payment.proto /app/gopay-flow/proto/payment.proto
COPY channels/gopay/app/*.py /app/gopay-app/
COPY channels/gopay/payment/gopay-flow/ /app/gopay-flow/
RUN cd /app/gopay-app \
    && python -m grpc_tools.protoc -I /app/proto --python_out=. --grpc_python_out=. /app/proto/gopay_app.proto \
    && cd /app/gopay-flow \
    && python -m grpc_tools.protoc -I proto --python_out=. --grpc_python_out=. proto/payment.proto

COPY gpt-service/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

EXPOSE 50051 50052 50054 8081
CMD ["/app/entrypoint.sh"]
