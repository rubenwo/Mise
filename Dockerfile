# syntax=docker/dockerfile:1

# Stage 1: Build React frontend
FROM node:24-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Stage 2: Build Go backend (with embedded frontend). cgo is needed for ONNX
# Runtime, so this builds against the same glibc as the runtime image.
FROM golang:1.26-trixie AS go-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
COPY --from=frontend-builder /app/frontend/dist ./internal/frontend/dist/
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -buildvcs=false -o /app/server ./cmd/server

# Stage 3: ONNX Runtime and EmbeddingGemma (int8) for in-process recipe
# embeddings, pinned by version and checksum.
FROM golang:1.26-trixie AS embedding
ADD --checksum=sha256:cc5c72baf5ae5c8238a6841f0897227be2d02826b9cf98eaf02fdefaeeb03a57 \
    https://github.com/microsoft/onnxruntime/releases/download/v1.31.0/onnxruntime-linux-x64-1.31.0.tgz /tmp/ort.tgz
RUN mkdir -p /out/lib && tar xzf /tmp/ort.tgz -C /tmp && \
    cp /tmp/onnxruntime-linux-x64-1.31.0/lib/libonnxruntime.so.1.31.0 /out/lib/libonnxruntime.so
ARG EG=https://huggingface.co/onnx-community/embeddinggemma-300m-ONNX/resolve/5090578d9565bb06545b4552f76e6bc2c93e4a66
ADD --checksum=sha256:172efde319fe1542dc41f31be6154910b05b78f7a861c265c4600eec906bd6d8 \
    ${EG}/onnx/model_quantized.onnx /out/models/embeddinggemma/model_quantized.onnx
ADD --checksum=sha256:705626e28e4c23c82ade34566b4197d97f534c12275fa406dfb71e9937d388c0 \
    ${EG}/onnx/model_quantized.onnx_data /out/models/embeddinggemma/model_quantized.onnx_data
ADD --checksum=sha256:4dda02faaf32bc91031dc8c88457ac272b00c1016cc679757d1c441b248b9c47 \
    ${EG}/tokenizer.json /out/models/embeddinggemma/tokenizer.json

# Stage 4: Minimal runtime (glibc + libstdc++ for ONNX Runtime, CA certs, tzdata).
# Runs as root so existing bind-mounted image volumes stay writable.
FROM gcr.io/distroless/cc-debian13
WORKDIR /app
COPY --from=embedding /out/lib/libonnxruntime.so /usr/lib/libonnxruntime.so
COPY --from=embedding /out/models /app/models
COPY --from=go-builder /app/server .
# config.yaml must be volume-mounted at /app/config.yaml
# recipe images are stored at /app/images (mount a persistent volume here)
EXPOSE 8080
ENTRYPOINT ["./server"]
