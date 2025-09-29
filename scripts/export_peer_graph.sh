#!/usr/bin/env bash
set -euo pipefail

OUTPUT="${OUTPUT:-peer_network.dot}"
PREFIX="${PREFIX:-peer}"
PEER_NETWORK="${PEER_NETWORK:-micro-net}"
EDGE1_NETWORK="${EDGE1_NETWORK:-edge-a-net}"
EDGE2_NETWORK="${EDGE2_NETWORK:-edge-b-net}"
ORIGIN_NAME="${ORIGIN_NAME:-origin-db}"
EDGE1_NAME="${EDGE1_NAME:-edge-db-a}"
EDGE2_NAME="${EDGE2_NAME:-edge-db-b}"

log() {
  printf '[%s] %s\n' "$(date +'%Y-%m-%dT%H:%M:%S%z')" "$*"
}

require_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    log "Docker CLI not available"
    exit 1
  fi
}

env_value() {
  local name="$1" key="$2"
  docker inspect "$name" \
    --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null |
    awk -F= -v key="$key" '$1==key {print $2}'
}

peer_neighbors() {
  local peer="$1"
  local neighbors
  neighbors=$(env_value "$peer" "PEER_NEIGHBORS")
  if [[ -z "$neighbors" ]]; then
    return
  fi
  IFS=',' read -ra arr <<<"$neighbors"
  for neighbor in "${arr[@]}"; do
    neighbor=$(echo "$neighbor" | xargs)
    [[ -n "$neighbor" ]] && printf '%s\n' "$neighbor"
  done
}

list_network_peers() {
  local net="$1"
  docker network inspect "$net" --format '{{range $id,$c := .Containers}}{{$c.Name}}{{"\n"}}{{end}}' 2>/dev/null |
    grep -E "^${PREFIX}-" || true
}

emit_header() {
  cat <<'EOF' >"$OUTPUT"
graph PeerMesh {
  graph [overlap=false, splines=true, fontsize=10, fontname="Arial"];
  node  [shape=circle, style=filled, fillcolor="#4058a9", fontcolor="#ffffff", fontname="Arial", width=0.5];
  edge  [color="#9fb1ff"];
EOF
}

emit_peer_nodes() {
  docker ps --filter "name=${PREFIX}-" --format '{{.Names}}' | while read -r peer; do
    [[ -z "$peer" ]] && continue
    printf '  "%s";\n' "$peer" >>"$OUTPUT"
  done
}

emit_peer_edges() {
  declare -A seen
  while read -r peer; do
    [[ -z "$peer" ]] && continue
    while read -r neighbor; do
      [[ -z "$neighbor" ]] && continue
      if [[ "$peer" < "$neighbor" ]]; then
        key="${peer}--${neighbor}"
      else
        key="${neighbor}--${peer}"
      fi
      if [[ -z "${seen[$key]:-}" ]]; then
        printf '  "%s" -- "%s";\n' "$peer" "$neighbor" >>"$OUTPUT"
        seen[$key]=1
      fi
    done < <(peer_neighbors "$peer")
  done < <(docker ps --filter "name=${PREFIX}-" --format '{{.Names}}')
}

emit_db_nodes() {
  cat <<EOF >>"$OUTPUT"
  "$ORIGIN_NAME" [shape=box, fillcolor="#f97838"];
  "$EDGE1_NAME" [shape=box, fillcolor="#f3c623"];
  "$EDGE2_NAME" [shape=box, fillcolor="#f3c623"];
  "$ORIGIN_NAME" -- "$EDGE1_NAME" [color="#f3c623", penwidth=2];
  "$ORIGIN_NAME" -- "$EDGE2_NAME" [color="#f3c623", penwidth=2];
EOF
}

emit_edge_links() {
  local net="$1" edge_name="$2" color="$3"
  while read -r peer; do
    [[ -z "$peer" ]] && continue
    printf '  "%s" -- "%s" [color="%s", style=dashed];\n' "$edge_name" "$peer" "$color" >>"$OUTPUT"
  done < <(list_network_peers "$net")
}

emit_footer() {
  echo "}" >>"$OUTPUT"
}

main() {
  require_docker
  emit_header
  emit_peer_nodes
  emit_peer_edges
  emit_db_nodes
  emit_edge_links "$EDGE1_NETWORK" "$EDGE1_NAME" "#ffda79"
  emit_edge_links "$EDGE2_NETWORK" "$EDGE2_NAME" "#ffda79"
  emit_footer
  log "GraphViz file written to $OUTPUT"
  log "Render with: dot -Tpng $OUTPUT -o peer_network.png (Graphviz required)"
}

main "$@"


