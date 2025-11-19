#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-peer-node:latest}"
ORIGIN_IMAGE="${ORIGIN_IMAGE:-origin-server:latest}"
EDGE_IMAGE="${EDGE_IMAGE:-edge-server:latest}"
COUNT="${COUNT:-30}"
MAX_COUNT="${MAX_COUNT:-160}"
PREFIX="${PREFIX:-peer}"
PEER_NETWORK="${PEER_NETWORK:-micro-net}"
EDGE1_NETWORK="${EDGE1_NETWORK:-edge-a-net}"
EDGE2_NETWORK="${EDGE2_NETWORK:-edge-b-net}"
ORIGIN_NETWORK="${ORIGIN_NETWORK:-origin-net}"
EDGE1_NAME="${EDGE1_NAME:-edge-1}"
EDGE2_NAME="${EDGE2_NAME:-edge-2}"
ORIGIN_NAME="${ORIGIN_NAME:-origin}"
PEER_PORT="${PEER_PORT:-8080}"
MEMORY_LIMIT="${MEMORY_LIMIT:-15m}"
CPU_LIMIT="${CPU_LIMIT:-0.05}"
PIDS_LIMIT="${PIDS_LIMIT:-50}"
TRACKER_URL="${TRACKER_URL:-http://tracker:7070}"
TOPOLOGY_URL="${TOPOLOGY_URL:-http://topology:8090}"
SIGNAL_URL="${SIGNAL_URL:-ws://signalling:7080/ws}"
PEER_ROOM="${PEER_ROOM:-default}"
PEER_REGION="${PEER_REGION:-global}"
PEER_RTT_MS="${PEER_RTT_MS:-25}"
CACHE_CAPACITY="${CACHE_CAPACITY:-64}"
HEARTBEAT_INTERVAL_SEC="${HEARTBEAT_INTERVAL_SEC:-30}"
CONNECTION_PROBABILITY="${CONNECTION_PROBABILITY:-0.3}"
EDGE_PEER_COUNT="${EDGE_PEER_COUNT:-5}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:16-alpine}"
POSTGRES_DB="${POSTGRES_DB:-hls}"
POSTGRES_USER="${POSTGRES_USER:-media}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-media_pass}"

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

build_origin_image() {
  if docker image inspect "$ORIGIN_IMAGE" >/dev/null 2>&1; then
    log "Origin image $ORIGIN_IMAGE already available"
    return
  fi
  log "Building origin image ($ORIGIN_IMAGE)"
  docker build -t "$ORIGIN_IMAGE" -f Dockerfile.origin .
}

build_edge_image() {
  if docker image inspect "$EDGE_IMAGE" >/dev/null 2>&1; then
    log "Edge image $EDGE_IMAGE already available"
    return
  fi
  log "Building edge image ($EDGE_IMAGE)"
  docker build -t "$EDGE_IMAGE" -f Dockerfile.edge .
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
  local ids
  ids=$(docker ps -aq --filter "name=${PREFIX}-" --filter "name=${ORIGIN_NAME}" --filter "name=${EDGE1_NAME}" --filter "name=${EDGE2_NAME}")
  if [[ -n "$ids" ]]; then
    log "Removing existing containers"
    docker rm -f $ids >/dev/null
  fi
}

# Generate adjacency matrix (30% connection probability)
# Uses bash random number generator
generate_adjacency_matrix() {
  local count="$1"
  local prob="$2"
  
  log "Generating adjacency matrix for $count peers with ${prob} connection probability"
  
  # Create a temporary file to store matrix
  local matrix_file=$(mktemp)
  
  # Initialize matrix structure (we'll output as neighbor lists per peer)
  # For simplicity, we'll generate neighbors directly
  echo "MATRIX_START"
  for i in $(seq 1 "$count"); do
    peer_a="${PREFIX}-${i}"
    neighbors=""
    for j in $(seq 1 "$count"); do
      if [[ $i -eq $j ]]; then
        continue
      fi
      # Generate random number 0-999, if < (prob * 1000) then connect
      # 30% = 300 out of 1000
      threshold=300
      random_val=$((RANDOM % 1000))
      if [[ $random_val -lt $threshold ]]; then
        peer_b="${PREFIX}-${j}"
        if [[ -z "$neighbors" ]]; then
          neighbors="$peer_b"
        else
          neighbors="$neighbors,$peer_b"
        fi
      fi
    done
    echo "$peer_a:$neighbors"
  done
  echo "MATRIX_END"
}

# Get neighbors for a peer from adjacency matrix
get_neighbors_from_matrix() {
  local peer_id="$1"
  local matrix_output="$2"
  
  echo "$matrix_output" | awk -F: -v peer="$peer_id" '
    /^MATRIX_START$/,/^MATRIX_END$/ {
      if ($1 == peer) {
        print $2
        exit
      }
    }
  '
}

start_postgres_for_origin() {
  local name="${ORIGIN_NAME}-db"
  local net="$ORIGIN_NETWORK"
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
    log "Container $name already exists, skipping"
    return
  fi
  log "Starting PostgreSQL container $name for origin"
  docker volume create "${name}-data" >/dev/null 2>&1 || true
  docker run -d \
    --name "$name" \
    --network "$net" \
    -e "POSTGRES_DB=$POSTGRES_DB" \
    -e "POSTGRES_USER=$POSTGRES_USER" \
    -e "POSTGRES_PASSWORD=$POSTGRES_PASSWORD" \
    -v "${name}-data:/var/lib/postgresql/data" \
    "$POSTGRES_IMAGE" >/dev/null
  sleep 2
}

start_postgres_for_edge() {
  local name="$1"
  local net="$2"
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
    log "Container $name already exists, skipping"
    return
  fi
  log "Starting PostgreSQL container $name for edge"
  docker volume create "${name}-data" >/dev/null 2>&1 || true
  docker run -d \
    --name "$name" \
    --network "$net" \
    -e "POSTGRES_DB=$POSTGRES_DB" \
    -e "POSTGRES_USER=$POSTGRES_USER" \
    -e "POSTGRES_PASSWORD=$POSTGRES_PASSWORD" \
    -v "${name}-data:/var/lib/postgresql/data" \
    "$POSTGRES_IMAGE" >/dev/null
  sleep 2
}

start_origin() {
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$ORIGIN_NAME"; then
    log "Origin $ORIGIN_NAME already exists, skipping"
    return
  fi
  log "Starting origin server $ORIGIN_NAME"
  docker run -d \
    --name "$ORIGIN_NAME" \
    --network "$ORIGIN_NETWORK" \
    -e "ORIGIN_PORT=8081" \
    -e "DB_HOST=${ORIGIN_NAME}-db" \
    -e "DB_PORT=5432" \
    -e "DB_USER=$POSTGRES_USER" \
    -e "DB_PASSWORD=$POSTGRES_PASSWORD" \
    -e "DB_NAME=$POSTGRES_DB" \
    -e "SONG_PATH=/home/origin/Rick-Roll-Sound-Effect.mp3" \
    "$ORIGIN_IMAGE" >/dev/null
}

start_edge() {
  local name="$1"
  local net="$2"
  local connected_peers="$3"
  local db_name="${name}-db"
  local origin_url="http://${ORIGIN_NAME}:8081"
  
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
    log "Edge $name already exists, skipping"
    return
  fi
  log "Starting edge server $name"
  docker run -d \
    --name "$name" \
    --network "$net" \
    -e "EDGE_NAME=$name" \
    -e "EDGE_PORT=8082" \
    -e "DB_HOST=$db_name" \
    -e "DB_PORT=5432" \
    -e "DB_USER=$POSTGRES_USER" \
    -e "DB_PASSWORD=$POSTGRES_PASSWORD" \
    -e "DB_NAME=$POSTGRES_DB" \
    -e "ORIGIN_URL=$origin_url" \
    -e "TOPOLOGY_URL=$TOPOLOGY_URL" \
    -e "CONNECTED_PEERS=$connected_peers" \
    "$EDGE_IMAGE" >/dev/null
}

start_peer() {
  local idx="$1"
  local name="${PREFIX}-${idx}"
  local neighbors="$2"
  local edge_urls="$3"
  
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
    log "Peer $name already exists, skipping"
    return
  fi
  log "Starting $name with neighbors: $neighbors"
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
    -e "TRACKER_URL=$TRACKER_URL" \
    -e "TOPOLOGY_URL=$TOPOLOGY_URL" \
    -e "SIGNAL_URL=$SIGNAL_URL" \
    -e "EDGE_URLS=$edge_urls" \
    -e "PEER_ROOM=$PEER_ROOM" \
    -e "PEER_REGION=$PEER_REGION" \
    -e "PEER_RTT_MS=$PEER_RTT_MS" \
    -e "CACHE_CAPACITY=$CACHE_CAPACITY" \
    -e "HEARTBEAT_INTERVAL_SEC=$HEARTBEAT_INTERVAL_SEC" \
    "$IMAGE" >/dev/null
}

connect_network() {
  local net="$1"
  local name="$2"
  if docker network inspect "$net" --format '{{range $id,$c := .Containers}}{{if eq $c.Name "'"$name"'"}}true{{end}}{{end}}' 2>/dev/null | grep -q "true"; then
    return
  fi
  log "Connecting $name to $net"
  docker network connect "$net" "$name" >/dev/null
}

# Select random peers for edge connection
select_edge_peers() {
  local count="$1"
  local total="$2"
  local selected=()
  
  while [[ ${#selected[@]} -lt "$count" ]]; do
    local peer_num=$((RANDOM % total + 1))
    local peer_name="${PREFIX}-${peer_num}"
    if [[ ! " ${selected[*]} " =~ " ${peer_name} " ]]; then
      selected+=("$peer_name")
    fi
  done
  
  IFS=,; echo "${selected[*]}"
}

# Register edge-peer connection in topology
register_edge_peer_connection() {
  local edge_name="$1"
  local peer_name="$2"
  
  # Wait a bit for topology service to be ready
  sleep 1
  
  # Update edge's neighbors in topology (append, not replace)
  # Get current neighbors first, then append
  current_neighbors=$(curl -s "$TOPOLOGY_URL/graph" 2>/dev/null | grep -o "\"$edge_name\":\[[^]]*\]" | grep -o '"[^"]*"' | tr '\n' ',' | sed 's/,$//' | sed 's/"//g' || echo "")
  
  if [[ -n "$current_neighbors" ]]; then
    neighbors_list="$current_neighbors,$peer_name"
  else
    neighbors_list="$peer_name"
  fi
  
  # Convert to JSON array format
  neighbors_json=$(echo "$neighbors_list" | awk -F',' '{
    printf "["
    for(i=1;i<=NF;i++) {
      if(i>1) printf ","
      printf "\"%s\"", $i
    }
    printf "]"
  }')
  
  payload=$(printf '{"peer_id":"%s","neighbors":%s}' "$edge_name" "$neighbors_json")
  curl -s -X POST "$TOPOLOGY_URL/edges" \
    -H "Content-Type: application/json" \
    -d "$payload" >/dev/null 2>&1 || true
}

main() {
  ensure_docker
  if [[ "$COUNT" -gt "$MAX_COUNT" ]]; then
    log "COUNT $COUNT greater than MAX_COUNT $MAX_COUNT, clamping"
    COUNT="$MAX_COUNT"
  fi

  build_peer_image
  build_origin_image
  build_edge_image
  
  ensure_network "$PEER_NETWORK"
  ensure_network "$EDGE1_NETWORK"
  ensure_network "$EDGE2_NETWORK"
  ensure_network "$ORIGIN_NETWORK"

  cleanup_existing_peers

  # Generate adjacency matrix
  log "Generating adjacency matrix..."
  ADJACENCY_MATRIX=$(generate_adjacency_matrix "$COUNT" "$CONNECTION_PROBABILITY")
  
  # Check if bc is available for probability calculation, if not use simpler method
  if ! command -v bc >/dev/null 2>&1; then
    log "bc not found, using simplified probability calculation"
    # Use integer math: 30% = 300 out of 1000
    CONNECTION_THRESHOLD=300
  else
    CONNECTION_THRESHOLD=$(echo "$CONNECTION_PROBABILITY * 1000" | bc | cut -d. -f1)
  fi
  
  # Start PostgreSQL databases
  start_postgres_for_origin
  start_postgres_for_edge "${EDGE1_NAME}-db" "$EDGE1_NETWORK"
  start_postgres_for_edge "${EDGE2_NAME}-db" "$EDGE2_NETWORK"
  
  # Start origin server
  start_origin
  
  # Connect origin to edge networks
  connect_network "$EDGE1_NETWORK" "$ORIGIN_NAME"
  connect_network "$EDGE2_NETWORK" "$ORIGIN_NAME"
  
  # Start edge servers (will be updated with peer connections after peers start)
  start_edge "$EDGE1_NAME" "$EDGE1_NETWORK" ""
  start_edge "$EDGE2_NAME" "$EDGE2_NETWORK" ""
  
  # Update edge config to include topology URL
  # Note: This requires restarting edges, but for now we'll pass it via env in start_edge
  
  # Connect edges to peer network
  connect_network "$PEER_NETWORK" "$EDGE1_NAME"
  connect_network "$PEER_NETWORK" "$EDGE2_NAME"
  
  # Select peers for each edge (before creating peers)
  EDGE1_PEERS=$(select_edge_peers "$EDGE_PEER_COUNT" "$COUNT")
  EDGE2_PEERS=$(select_edge_peers "$EDGE_PEER_COUNT" "$COUNT")
  
  log "Edge $EDGE1_NAME will connect to peers: $EDGE1_PEERS"
  log "Edge $EDGE2_NAME will connect to peers: $EDGE2_PEERS"
  
  # Prepare edge URLs for peers
  EDGE_URLS="http://${EDGE1_NAME}:8082,http://${EDGE2_NAME}:8082"
  
  # Start peers with adjacency matrix neighbors FIRST
  # Note: Edges are NOT added to PEER_NEIGHBORS - they are accessed via EDGE_URLS only
  log "Starting $COUNT peers with adjacency matrix topology..."
  for i in $(seq 1 "$COUNT"); do
    peer_id="${PREFIX}-${i}"
    neighbors=$(get_neighbors_from_matrix "$peer_id" "$ADJACENCY_MATRIX")
    
    # Edges are connected via network but NOT as P2P neighbors
    # They are accessed through EDGE_URLS for segment requests
    # Network connection is handled separately via connect_network calls
    
    start_peer "$i" "$neighbors" "$EDGE_URLS"
    sleep 0.05
  done
  
  # Wait a bit for peers to start
  sleep 2
  
  # Now connect selected peers to edge networks
  IFS=',' read -ra PEERS1 <<<"$EDGE1_PEERS"
  for peer in "${PEERS1[@]}"; do
    connect_network "$EDGE1_NETWORK" "$peer"
  done
  
  IFS=',' read -ra PEERS2 <<<"$EDGE2_PEERS"
  for peer in "${PEERS2[@]}"; do
    connect_network "$EDGE2_NETWORK" "$peer"
  done
  
  # Update edge containers with connected peers (restart with new env)
  log "Updating edge servers with connected peers..."
  docker stop "$EDGE1_NAME" >/dev/null 2>&1 || true
  docker rm "$EDGE1_NAME" >/dev/null 2>&1 || true
  start_edge "$EDGE1_NAME" "$EDGE1_NETWORK" "$EDGE1_PEERS"
  
  docker stop "$EDGE2_NAME" >/dev/null 2>&1 || true
  docker rm "$EDGE2_NAME" >/dev/null 2>&1 || true
  start_edge "$EDGE2_NAME" "$EDGE2_NETWORK" "$EDGE2_PEERS"
  
  # Reconnect edges to networks
  connect_network "$PEER_NETWORK" "$EDGE1_NAME"
  connect_network "$PEER_NETWORK" "$EDGE2_NAME"
  connect_network "$EDGE1_NETWORK" "$ORIGIN_NAME"
  connect_network "$EDGE2_NETWORK" "$ORIGIN_NAME"

  log "Peer network ready (${COUNT} peers)"
  log "Origin: $ORIGIN_NAME | Edges: $EDGE1_NAME, $EDGE2_NAME"
  log "Adjacency matrix: ${CONNECTION_PROBABILITY} connection probability"
}

main "$@"

