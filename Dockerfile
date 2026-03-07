# syntax=docker.io/docker/dockerfile:1
# ── ai-workflow production image ──
# Multi-stage: build with Go+Node, run on alpine (~30MB)

# ═══ Stage 1: Build ═══
FROM golang:1.25-bookworm AS builder

RUN apt-get update \
    && apt-get install -y --no-install-recommends git curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ARG NODE_MAJOR=22
RUN curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
        | gpg --dearmor -o /usr/share/keyrings/nodesource.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
        > /etc/apt/sources.list.d/nodesource.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

ENV GOPROXY=https://goproxy.cn,direct
RUN npm config set registry https://registry.npmmirror.com

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY web/package.json web/package-lock.json ./web/
RUN npm --prefix web ci

COPY . .

RUN npm --prefix web run build

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o /ai-flow ./cmd/ai-flow

# ═══ Stage 2: Runtime (alpine, ~30MB) ═══
FROM alpine:3.21

RUN apk add --no-cache ca-certificates git sqlite

COPY --from=builder /ai-flow /usr/local/bin/ai-flow
COPY --from=builder /build/web/dist /opt/ai-workflow/web/dist

ENV AI_WORKFLOW_DATA_DIR=/data
ENV AI_WORKFLOW_FRONTEND_DIR=/opt/ai-workflow/web/dist
ENV AI_WORKFLOW_SERVER_HOST=0.0.0.0

WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["ai-flow"]
CMD ["server", "--port", "8080"]
