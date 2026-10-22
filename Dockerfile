# Stage 1: Build React frontend
FROM node:24-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Stage 2: Build Go backend (with embedded frontend)
FROM golang:1.26-alpine AS go-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
COPY --from=frontend-builder /app/frontend/dist ./internal/frontend/dist/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -buildvcs=false -o /app/server ./cmd/server

# Stage 3: Minimal runtime (static binary + CA certs + tzdata).
# Runs as root so existing bind-mounted image volumes stay writable.
FROM gcr.io/distroless/static-debian13
WORKDIR /app
COPY --from=go-builder /app/server .
# config.yaml must be volume-mounted at /app/config.yaml
# recipe images are stored at /app/images (mount a persistent volume here)
EXPOSE 8080
ENTRYPOINT ["./server"]
