## Peer Network & Edge Databases

Use the helper script to spin up up to 160 Go-based peer containers (default 160) that stay under 15 MB RAM and use minimal CPU. Each peer runs a lightweight Go agent that forms a partial mesh with its neighbors. Two beefier PostgreSQL “edge” nodes and one “origin” node are also provisioned for storing song HLS segments.

## Signalling Server (Shortest-Path Aware)

`cmd/signalling` exposes a WebSocket endpoint (`/ws?peer=<name>&room=<interest>`) that:
- accepts neighbor announcements from peer containers,
- computes shortest paths in each room (BFS over the announced edges),
- broadcasts the resulting hop-by-hop route so data always flows along existing links (no direct shortcuts).

### Run
```bash
go run ./cmd/signalling
# listens on :7080 (override via SIGNAL_ADDR)
```

### Peer Message Types
- `announce`  
  `{"type":"announce","peer":"peer-1","room":"rock","neighbors":["peer-2","peer-5"]}`
- `request_path`  
  `{"type":"request_path","target":"peer-30"}`

When a path is available, every peer on the path receives:
`{"type":"path","path":["peer-1","peer-6","peer-18","peer-30"]}` enabling A→B→C relays without bypassing intermediate nodes.

### Prerequisites
- Docker Engine 20.10+
- Bash shell (Git Bash, WSL, Linux, or macOS terminal)

### Launch Peer Mesh + Databases
```bash
chmod +x scripts/deploy_peer_network.sh
scripts/deploy_peer_network.sh
```

Key environment overrides:
- `COUNT` (default `160`, clamped by `MAX_COUNT`)
- `PREFIX` (default `peer`)
- `PEER_NETWORK`, `EDGE1_NETWORK`, `EDGE2_NETWORK`, `ORIGIN_NETWORK`
- `EDGE1_PEERS`, `EDGE2_PEERS` limit which peers connect to each edge
- `IMAGE` (default `peer-node:latest`, built from `Dockerfile.peer`)
- Postgres-specific: `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`

Example (80 peers, custom edge peer sets):
```bash
COUNT=80 \
EDGE1_PEERS="peer-1,peer-2,peer-3" \
EDGE2_PEERS="peer-40,peer-41,peer-42" \
scripts/deploy_peer_network.sh
```

### Distribute Image Without Re-Pulling
Pull once, export, copy, and load on other machines:
```bash
docker pull busybox:stable-musl
docker save busybox:stable-musl -o busybox.tar
# copy busybox.tar to other hosts
docker load -i busybox.tar
```

### Monitoring & Cleanup
- Peers: `docker ps | grep peer-`
- Network graph: `docker network inspect micro-net`
- Edge scope: `docker network inspect edge-a-net`
- Tear down peers: `docker rm -f $(docker ps -aq --filter "name=peer-")`
- Remove databases: `docker rm -f edge-db-a edge-db-b origin-db`

### Visualize the Mesh
Generate a Graphviz representation and render it (Graphviz `dot` must be installed):
```bash
chmod +x scripts/export_peer_graph.sh
scripts/export_peer_graph.sh      # writes peer_network.dot
dot -Tpng peer_network.dot -o peer_network.png
```

### Legacy Lightweight Containers
The original BusyBox-only launcher is still available at `scripts/deploy_500_containers.sh` if you only need capped containers without the Go peer agent or database tier.

