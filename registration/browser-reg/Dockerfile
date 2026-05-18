FROM nb-register-camoufox-base:latest

# --- Application ---
COPY registration/browser-reg /app
COPY proto/browser.proto /app/proto/browser.proto
WORKDIR /app

# --- Compile proto ---
RUN python -m grpc_tools.protoc -I proto --python_out=. --grpc_python_out=. proto/browser.proto

# Screenshot output
RUN mkdir -p /tmp/screenshots
ENV SCREENSHOT_DIR=/tmp/screenshots

# Default: gRPC service mode
ENV GRPC_PORT=50051
EXPOSE 50051

ENTRYPOINT ["python", "main.py", "serve"]
