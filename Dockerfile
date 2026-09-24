# ---- Frontend build stage ----
FROM node:22-alpine AS frontend

WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- Go build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /app/frontend/build ./frontend/build
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# ---- Production stage ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates curl

# A numeric UID: Kubernetes can only verify runAsNonRoot against a number, and
# k8s/base/deployment.yaml pins the same 1001.
RUN addgroup -S -g 1001 appgroup && adduser -S -u 1001 -G appgroup appuser

COPY --from=builder /app/server /server

USER 1001:1001

EXPOSE 3000

CMD ["/server"]
