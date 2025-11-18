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

### Containerized Control Plane

Run Redis + topology + tracker + signalling inside Docker so peers can talk to them over the shared `micro-net` bridge:

```bash
docker compose -f docker-compose.control.yml up -d --build
# brings up redis, topology (:8090), tracker (:7070), signalling (:7080)
```

All services join the `micro-net` network that the peers use, so peers can reach them via the hostnames `redis`, `topology`, `tracker`, and `signalling`. Stop everything with `docker compose -f docker-compose.control.yml down`.

## Tracker Service (Redis + TTL Heartbeats)

`cmd/tracker` maintains the live segment registry with Redis:
- Peers `POST /announce` with their region, RTT estimate, neighbor set, and owned segments.
- `POST /heartbeat` (every <120 s) refreshes the TTL and optionally updates segments or neighbors.
- `GET /segments/<segment>?region=<preferred>` lists candidate peers sorted by region proximity and RTT.
- Background reaper drops peers that miss the TTL, cleans Redis entries, and notifies the topology manager so edges disappear immediately.

Run (Redis required):
```bash
REDIS_ADDR=localhost:6379 TOPOLOGY_URL=http://localhost:8090 go run ./cmd/tracker
```

Tunables: `TRACKER_ADDR`, `TRACKER_TTL_SECONDS`, `TOPOLOGY_URL`.

## Topology Manager

`cmd/topology` is the global network brain; it stores node metadata plus the undirected connection graph.

- `POST /peers` upserts peer info (`peer_id`, `neighbors`, optional region/RTT/metadata).
- `DELETE /peers/<id>` removes a peer and all incident edges (used by the tracker on TTL expiry).
- `GET /graph` returns the adjacency snapshot; `GET /path?from=A&to=B` runs a BFS for orchestration tooling.

Run:
```bash
go run ./cmd/topology
```

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
- Peer runtime: `TRACKER_URL`, `SIGNAL_URL`, `PEER_ROOM`, `PEER_REGION`, `PEER_RTT_MS`, `CACHE_CAPACITY`, `HEARTBEAT_INTERVAL_SEC`
  - Defaults assume the Docker Compose control plane (`TRACKER_URL=http://tracker:7070`, `SIGNAL_URL=ws://signalling:7080/ws`). Override them (e.g., `host.docker.internal`) if you run services on the host instead.

Example (80 peers, custom edge peer sets):
```bash
COUNT=80 \
EDGE1_PEERS="peer-1,peer-2,peer-3" \
EDGE2_PEERS="peer-40,peer-41,peer-42" \
scripts/deploy_peer_network.sh
```

Each peer container now includes:
- LRU cache for HLS segments (capacity via `CACHE_CAPACITY`, default 64 entries).
- REST endpoints for ingesting and serving segments (`POST /segments`, `GET /segments/{id}`).
- Automatic tracker announce/heartbeat cycles carrying the full segment inventory and neighbor list.
- Optional signalling client to publish neighbors and receive hop-by-hop routes that keep transfers constrained to the mesh.

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

Tracker TTL cleanup + topology manager updates ensure expired peers are removed before exporting, keeping the diagram accurate in near real-time.

### Legacy Lightweight Containers
The original BusyBox-only launcher is still available at `scripts/deploy_500_containers.sh` if you only need capped containers without the Go peer agent or database tier.

