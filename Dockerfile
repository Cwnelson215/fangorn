# ---- Frontend build stage ----
FROM node:20-alpine AS frontend

WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- Go build stage ----
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /app/frontend/build ./frontend/build
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# ---- Production stage ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates curl

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /app/server /server

USER appuser

EXPOSE 3000

CMD ["/server"]
