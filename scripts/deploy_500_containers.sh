#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-busybox:stable-musl}"
COUNT="${COUNT:-160}"
MAX_COUNT="${MAX_COUNT:-160}"
NETWORK="${NETWORK:-micro-net}"
PREFIX="${PREFIX:-micro}"
CPU_LIMIT="${CPU_LIMIT:-0.05}"
MEMORY_LIMIT="${MEMORY_LIMIT:-15m}"
PIDS_LIMIT="${PIDS_LIMIT:-20}"
START_DELAY="${START_DELAY:-0.05}"
COMMAND="${COMMAND:-sleep infinity}"

log() {
  printf '[%s] %s\n' "$(date +'%Y-%m-%dT%H:%M:%S%z')" "$*"
}

ensure_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    log "Docker CLI not found"
    exit 1
  fi
}

pull_image_once() {
  if docker image inspect "$IMAGE" >/dev/null 2>&1; then
    log "Image $IMAGE already present locally"
  else
    log "Pulling image $IMAGE"
    docker pull "$IMAGE"
  fi
}

ensure_network() {
  local net="$1"
  if docker network inspect "$net" >/dev/null 2>&1; then
    log "Network $net already exists"
  else
    log "Creating network $net"
    docker network create "$net" >/dev/null
  fi
}

start_container() {
  local idx="$1"
  local name="${PREFIX}-${idx}"

  if docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
    log "Container $name already exists, skipping"
    return
  fi

  log "Starting $name"
  docker run -d \
    --name "$name" \
    --network "$NETWORK" \
    --memory "$MEMORY_LIMIT" \
    --memory-swap "$MEMORY_LIMIT" \
    --cpus "$CPU_LIMIT" \
    --pids-limit "$PIDS_LIMIT" \
    "$IMAGE" \
    $COMMAND >/dev/null
}

main() {
  ensure_docker
  if [ "$COUNT" -gt "$MAX_COUNT" ]; then
    log "Requested COUNT ($COUNT) exceeds MAX_COUNT ($MAX_COUNT); clamping"
    COUNT="$MAX_COUNT"
  fi
  pull_image_once
  ensure_network "$NETWORK"

  for i in $(seq 1 "$COUNT"); do
    start_container "$i"
    sleep "$START_DELAY"
  done

  log "Launched $COUNT containers with prefix $PREFIX"
}

main "$@"

