FROM docker.m.daocloud.io/library/golang:1.26-alpine AS builder

WORKDIR /app

# Enable go module proxy
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
    && protoc -I proto --go_out=pb --go-grpc_out=pb proto/*.proto

# Build the whole package. Building main.go alone misses workflow/activity files
# that live in the same package.
RUN go build -o orchestrator .

FROM docker.m.daocloud.io/library/alpine:latest

# Use domestic apk mirrors
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

WORKDIR /app
COPY --from=builder /app/orchestrator .

EXPOSE 50051

CMD ["./orchestrator"]
