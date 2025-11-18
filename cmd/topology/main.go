package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"cloud_project/internal/topology"
)

type upsertRequest struct {
	PeerID    string         `json:"peer_id"`
	Region    string         `json:"region"`
	RTTms     int            `json:"rtt_ms"`
	Neighbors []string       `json:"neighbors"`
	Metadata  map[string]any `json:"metadata"`
}

func main() {
	addr := env("TOPOLOGY_ADDR", ":8090")
	graph := topology.NewGraph()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req upsertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.PeerID == "" {
			http.Error(w, "peer_id required", http.StatusBadRequest)
			return
		}
		graph.Upsert(req.PeerID, req.Region, req.RTTms, req.Neighbors, req.Metadata)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/peers/", func(w http.ResponseWriter, r *http.Request) {
		peerID := strings.TrimPrefix(r.URL.Path, "/peers/")
		if peerID == "" {
			http.Error(w, "peer id required", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		graph.Remove(peerID)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/graph", func(w http.ResponseWriter, r *http.Request) {
		topology.WriteJSON(w, http.StatusOK, graph.Snapshot())
	})
	mux.HandleFunc("/graph/ui", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		// Simple D3 force-directed graph that fetches /graph JSON from this service
		_, _ = w.Write([]byte(`<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Peer Graph Visualization</title>
  <script src="https://d3js.org/d3.v7.min.js"></script>
  <style>
    body {
      margin: 0;
      font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: #050816;
      color: #e5e7eb;
    }
    header {
      padding: 12px 20px;
      border-bottom: 1px solid #1f2933;
      background: radial-gradient(circle at top, #111827 0, #020617 55%);
    }
    h1 {
      font-size: 18px;
      margin: 0 0 4px 0;
    }
    .subtitle {
      font-size: 13px;
      color: #9ca3af;
    }
    #graph {
      width: 100vw;
      height: calc(100vh - 60px);
    }
    .link {
      stroke: #4b5563;
      stroke-opacity: 0.7;
      stroke-width: 1.4px;
    }
    .node circle {
      stroke: #0f172a;
      stroke-width: 1.5px;
      cursor: pointer;
    }
    .node text {
      font-size: 11px;
      fill: #e5e7eb;
      pointer-events: none;
    }
    .node--selected circle {
      stroke: #facc15;
      stroke-width: 3px;
    }
    .tooltip {
      position: fixed;
      background: #020617;
      color: #e5e7eb;
      padding: 6px 10px;
      border-radius: 6px;
      border: 1px solid #1f2937;
      font-size: 12px;
      pointer-events: none;
      opacity: 0;
      transition: opacity 0.15s ease;
      white-space: nowrap;
      z-index: 50;
    }
  </style>
</head>
<body>
  <header>
    <h1>Peer Network Graph</h1>
    <div class="subtitle">
      Data from <code>/graph</code> &mdash; nodes are peers, edges are neighbor links.
      <button id="refreshBtn" style="margin-left:12px;padding:4px 10px;border-radius:4px;border:none;background:#2563eb;color:white;cursor:pointer;">Refresh Now</button>
    </div>
  </header>
  <svg id="graph"></svg>
  <div id="tooltip" class="tooltip"></div>
  <script>
    const svg = d3.select("#graph");
    const width = window.innerWidth;
    const height = window.innerHeight - 60;
    svg.attr("width", width).attr("height", height);

    const tooltip = document.getElementById("tooltip");
    const linkLayer = svg.append("g").attr("stroke-linecap", "round");
    const nodeLayer = svg.append("g");
    let linkSelection = linkLayer.selectAll("line");
    let nodeSelection = nodeLayer.selectAll("g");
    const canonicalOrder = [];

    function showTooltip(evt, text) {
      tooltip.textContent = text;
      tooltip.style.left = (evt.clientX + 10) + "px";
      tooltip.style.top = (evt.clientY + 10) + "px";
      tooltip.style.opacity = "1";
    }
    function hideTooltip() {
      tooltip.style.opacity = "0";
    }

    function keyForLink(d) {
      const src = d.source.id || d.source;
      const tgt = d.target.id || d.target;
      return src < tgt ? src + "-" + tgt : tgt + "-" + src;
    }

    function canonicalKey(a, b) {
      return a < b ? a + "::" + b : b + "::" + a;
    }

    function renderGraph(data) {
      const nodesMap = new Map();
      const linkMap = new Map();
      console.log(data);

      Object.entries(data).forEach(([peer, neighbors]) => {
        if (!nodesMap.has(peer)) nodesMap.set(peer, { id: peer });
        neighbors.forEach(n => {
          if (!nodesMap.has(n)) nodesMap.set(n, { id: n });
          const key = canonicalKey(peer, n);
          if (!linkMap.has(key)) {
            linkMap.set(key, { source: peer, target: n });
          }
        });
      });

      const links = Array.from(linkMap.values());

      const nodes = Array.from(nodesMap.values()).sort((a, b) => {
        const aNum = parseInt(a.id.replace(/[^0-9]/g, ""), 10);
        const bNum = parseInt(b.id.replace(/[^0-9]/g, ""), 10);
        if (Number.isNaN(aNum) || Number.isNaN(bNum)) {
          return a.id.localeCompare(b.id);
        }
        return aNum - bNum;
      });

      nodes.forEach(node => {
        if (!canonicalOrder.includes(node.id)) {
          canonicalOrder.push(node.id);
        }
      });
      const centerX = width / 2;
      const centerY = height / 2;
      const radius = Math.min(width, height) / 2 - 80;
      const count = canonicalOrder.length || 1;
      const positionMap = new Map();
      canonicalOrder.forEach((id, idx) => {
        const angle = (2 * Math.PI * idx) / count;
        positionMap.set(id, {
          x: centerX + radius * Math.cos(angle),
          y: centerY + radius * Math.sin(angle),
        });
      });
      nodes.forEach(node => {
        const pos = positionMap.get(node.id);
        node.x = pos?.x ?? centerX;
        node.y = pos?.y ?? centerY;
      });

      const nodeById = new Map(nodes.map(n => [n.id, n]));
      const resolvedLinks = links
        .map(l => {
          const source = nodeById.get(l.source);
          const target = nodeById.get(l.target);
          if (!source || !target) return null;
          return { source, target };
        })
        .filter(Boolean);

      linkSelection = linkSelection
        .data(resolvedLinks, keyForLink);
      linkSelection.exit().remove();
      const linkEnter = linkSelection.enter()
        .append("line")
        .attr("class", "link");
      linkSelection = linkEnter.merge(linkSelection);

      nodeSelection = nodeSelection
        .data(nodes, d => d.id);

      nodeSelection.exit().remove();

      const nodeEnter = nodeSelection.enter()
        .append("g")
        .attr("class", "node");

      nodeEnter.append("circle")
        .attr("r", 10)
        .attr("fill", d => d.id.startsWith("peer-") ? "#22c55e" : "#38bdf8")
        .on("mouseover", (evt, d) => showTooltip(evt, d.id))
        .on("mouseout", hideTooltip)
        .on("click", (_, d) => {
          svg.selectAll(".node").classed("node--selected", n => n.id === d.id);
        });

      nodeEnter.append("text")
        .attr("x", 0)
        .attr("y", 22)
        .attr("text-anchor", "middle")
        .text(d => d.id);

      nodeSelection = nodeEnter.merge(nodeSelection);
      nodeSelection.attr("transform", d => "translate(" + d.x + "," + d.y + ")");

      linkSelection
        .attr("x1", d => d.source.x)
        .attr("y1", d => d.source.y)
        .attr("x2", d => d.target.x)
        .attr("y2", d => d.target.y);
    }

    function fetchGraph() {
      fetch("/graph?ts=" + Date.now())
        .then(r => r.json())
        .then(renderGraph)
        .catch(err => {
          console.error("Failed to load /graph:", err);
        });
    }

    document.getElementById("refreshBtn").addEventListener("click", fetchGraph);
    fetchGraph();
  </script>
</body>
</html>`))
	})
	mux.HandleFunc("/path", func(w http.ResponseWriter, r *http.Request) {
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		if from == "" || to == "" {
			http.Error(w, "from/to required", http.StatusBadRequest)
			return
		}
		path, err := graph.BFS(from, to)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		topology.WriteJSON(w, http.StatusOK, map[string]any{
			"path": path,
		})
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	log.Printf("Topology manager listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("topology server error: %v", err)
	}
}

func env(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}
