# Stage 1: Build React frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Stage 2: Build Go backend (with embedded frontend)
FROM golang:1.26-bookworm AS go-builder
WORKDIR /app
COPY backend/ ./backend/
COPY --from=frontend-builder /app/frontend/dist ./backend/internal/frontend/dist/
WORKDIR /app/backend
RUN go build -buildvcs=false -o /app/server ./cmd/server

# Stage 3: Minimal runtime
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=go-builder /app/server .
# config.yaml must be volume-mounted at /app/config.yaml
# recipe images are stored at /app/images (mount a persistent volume here)
EXPOSE 8080
ENTRYPOINT ["./server"]
