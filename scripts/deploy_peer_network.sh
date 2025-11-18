#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-peer-node:latest}"
COUNT="${COUNT:-160}"
MAX_COUNT="${MAX_COUNT:-160}"
PREFIX="${PREFIX:-peer}"
PEER_NETWORK="${PEER_NETWORK:-micro-net}"
EDGE1_NETWORK="${EDGE1_NETWORK:-edge-a-net}"
EDGE2_NETWORK="${EDGE2_NETWORK:-edge-b-net}"
ORIGIN_NETWORK="${ORIGIN_NETWORK:-origin-net}"
EDGE1_NAME="${EDGE1_NAME:-edge-db-a}"
EDGE2_NAME="${EDGE2_NAME:-edge-db-b}"
ORIGIN_NAME="${ORIGIN_NAME:-origin-db}"
EDGE1_PEERS="${EDGE1_PEERS:-peer-1,peer-2,peer-3,peer-4,peer-5,peer-6,peer-7,peer-8}"
EDGE2_PEERS="${EDGE2_PEERS:-peer-20,peer-21,peer-22,peer-23,peer-24,peer-25,peer-26,peer-27}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:16-alpine}"
POSTGRES_DB="${POSTGRES_DB:-hls}"
POSTGRES_USER="${POSTGRES_USER:-media}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-media_pass}"
RESET_PEERS="${RESET_PEERS:-true}"
PEER_PORT="${PEER_PORT:-8080}"
MEMORY_LIMIT="${MEMORY_LIMIT:-15m}"
CPU_LIMIT="${CPU_LIMIT:-0.05}"
PIDS_LIMIT="${PIDS_LIMIT:-50}"

log() {
  printf '[%s] %s\n' "$(date +'%Y-%m-%dT%H:%M:%S%z')" "$*"
}

ensure_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    log "Docker CLI not found"
    exit 1
  fi
}

build_peer_image() {
  if docker image inspect "$IMAGE" >/dev/null 2>&1; then
    log "Peer image $IMAGE already available"
    return
  fi
  log "Building peer image ($IMAGE)"
  docker build -t "$IMAGE" -f Dockerfile.peer .
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

cleanup_existing_peers() {
  if [[ "$RESET_PEERS" != "true" ]]; then
    return
  fi
  local ids
  ids=$(docker ps -aq --filter "name=${PREFIX}-")
  if [[ -n "$ids" ]]; then
    log "Removing existing peer containers"
    docker rm -f $ids >/dev/null
  fi
}

neighbors_for_index() {
  local idx="$1"
  local total="$2"
  local neighbors=()
  local prev=$(( (idx + total - 2) % total + 1 ))
  local next=$(( idx % total + 1 ))
  local skip=$(( (idx + 4 - 1) % total + 1 ))
  neighbors+=("${PREFIX}-${prev}")
  neighbors+=("${PREFIX}-${next}")
  neighbors+=("${PREFIX}-${skip}")
  local unique=()
  for n in "${neighbors[@]}"; do
    if [[ " ${unique[*]} " != *" $n "* ]]; then
      unique+=("$n")
    fi
  done
  IFS=,; echo "${unique[*]}"
}

start_peer() {
  local idx="$1"
  local name="${PREFIX}-${idx}"
  local neighbors
  neighbors=$(neighbors_for_index "$idx" "$COUNT")
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
    log "Peer $name already exists, skipping"
    return
  fi
  log "Starting $name with neighbors [$neighbors]"
  docker run -d \
    --name "$name" \
    --network "$PEER_NETWORK" \
    --memory "$MEMORY_LIMIT" \
    --memory-swap "$MEMORY_LIMIT" \
    --cpus "$CPU_LIMIT" \
    --pids-limit "$PIDS_LIMIT" \
    -e "PEER_NAME=$name" \
    -e "PEER_PORT=$PEER_PORT" \
    -e "PEER_NEIGHBORS=$neighbors" \
    "$IMAGE" >/dev/null
}

network_has_container() {
  local net="$1"
  local name="$2"
  local present
  present=$(docker network inspect "$net" --format '{{range $id,$c := .Containers}}{{if eq $c.Name "'"$name"'"}}true{{end}}{{end}}' 2>/dev/null)
  [[ "$present" == "true" ]]
}

connect_network() {
  local net="$1"
  local name="$2"
  if network_has_container "$net" "$name"; then
    return
  fi
  log "Connecting $name to $net"
  docker network connect "$net" "$name" >/dev/null
}

start_postgres() {
  local name="$1"
  local net="$2"
  local volume="$3"
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
    log "Container $name already exists, skipping"
    return
  fi
  log "Starting PostgreSQL container $name"
  docker volume create "$volume" >/dev/null 2>&1 || true
  docker run -d \
    --name "$name" \
    --network "$net" \
    -e "POSTGRES_DB=$POSTGRES_DB" \
    -e "POSTGRES_USER=$POSTGRES_USER" \
    -e "POSTGRES_PASSWORD=$POSTGRES_PASSWORD" \
    -v "${volume}:/var/lib/postgresql/data" \
    "$POSTGRES_IMAGE" >/dev/null
}

connect_edge_peers() {
  local net="$1"
  local peers_csv="$2"
  IFS=',' read -ra peers <<<"$peers_csv"
  for peer in "${peers[@]}"; do
    peer=$(echo "$peer" | xargs)
    if [[ -z "$peer" ]]; then
      continue
    fi
    connect_network "$net" "$peer"
  done
}

main() {
  ensure_docker
  if [[ "$COUNT" -gt "$MAX_COUNT" ]]; then
    log "COUNT $COUNT greater than MAX_COUNT $MAX_COUNT, clamping"
    COUNT="$MAX_COUNT"
  fi

  build_peer_image
  ensure_network "$PEER_NETWORK"
  ensure_network "$EDGE1_NETWORK"
  ensure_network "$EDGE2_NETWORK"
  ensure_network "$ORIGIN_NETWORK"

  cleanup_existing_peers

  for i in $(seq 1 "$COUNT"); do
    start_peer "$i"
    sleep 0.05
  done

  start_postgres "$ORIGIN_NAME" "$ORIGIN_NETWORK" "${ORIGIN_NAME}-data"
  start_postgres "$EDGE1_NAME" "$ORIGIN_NETWORK" "${EDGE1_NAME}-data"
  start_postgres "$EDGE2_NAME" "$ORIGIN_NETWORK" "${EDGE2_NAME}-data"

  connect_network "$EDGE1_NETWORK" "$EDGE1_NAME"
  connect_network "$EDGE2_NETWORK" "$EDGE2_NAME"

  connect_edge_peers "$EDGE1_NETWORK" "$EDGE1_PEERS"
  connect_edge_peers "$EDGE2_NETWORK" "$EDGE2_PEERS"

  log "Peer network ready (${COUNT} peers)"
  log "Origin: $ORIGIN_NAME | Edges: $EDGE1_NAME, $EDGE2_NAME"
}

main "$@"

