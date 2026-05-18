FROM docker.m.daocloud.io/library/python:3.12-slim
WORKDIR /app
RUN pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple grpcio grpcio-tools "protobuf>=6.31.1" "requests[socks]" "psycopg[binary]"
COPY proto/gopay_app.proto /app/proto/gopay_app.proto
RUN python -m grpc_tools.protoc -I /app/proto --python_out=. --grpc_python_out=. /app/proto/gopay_app.proto
COPY channels/gopay/app/*.py ./
ENV PYTHONUNBUFFERED=1
EXPOSE 50051
CMD ["python3", "app_server.py"]
