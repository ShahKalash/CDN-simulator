CDN Simulator — Build Checklist (Go)

Master Checklist

- [ ] Define scope, metrics (hit ratio, p95/p99 latency, origin offload, bandwidth cost), scenarios
- [ ] Initialize Go module and scaffold project structure
- [ ] Implement discrete-event simulation core (clock, event queue, RNG)
- [ ] Model topology, links, latency/bandwidth; shortest-path utilities
- [ ] Implement cache interfaces and policies (LRU, LFU, TinyLFU/admission)
- [ ] Implement CDN node request pipeline and background tasks (fill, refresh, purge)
- [ ] Implement routing strategies (nearest latency, consistent hashing, multi-tier)
- [ ] Add workload generators and content catalog (Zipf, TTL, sizes)
- [ ] Model origin capacity and latency distribution
- [ ] Implement consistency, TTL, invalidation, stale-while-revalidate
- [ ] Metrics, tracing, and reporting (CSV/JSON exports)
- [ ] CLI and scenario runner (config, sweep, seed, output)
- [ ] Example scenarios/configs and documentation
- [ ] CI with tests and benchmarks; profiling and optimization

Day-by-Day Plan (2 weeks)

Day 1 — Scope & Design

- Define goals, KPIs, and success criteria
- Draft architecture: packages sim, topology, cache, node, routing, workload, metrics, config, origin, cmd/simulator
- Decide config format (YAML/JSON), RNG seeding, determinism

Day 2 — Project Scaffolding (Go)

- `go mod init`, directory layout, basic logging
- Add Makefile or task runner, unit test skeleton
- Create `cmd/simulator` stub with `--config`, `--seed`

Day 3 — Config & Models

- Define config structs and loader + validation
- Model content item (id, size, ttl), workload params, topology, cache sizes

Day 4 — Simulation Core

- Implement event queue, simulation clock, RNG utils
- Event types: request arrival, network complete, cache expiry, invalidation

Day 5 — Topology & Network

- Graph of nodes (edge, regional, origin); links with latency/bw
- Path/latency calculators; simple bandwidth contention model (optional queues)

Day 6 — Cache Subsystem

- `Cache` interface; LRU baseline with size awareness
- Miss handling hooks; microbenchmarks

Day 7 — Cache Policies

- Implement LFU and TinyLFU (admission filter)
- Add TTL handling and capacity by bytes

Day 8 — CDN Node Behavior

- Request pipeline: route → cache lookup → parent/origin fetch → fill → serve
- Background: refresh-on-expire, purge handling

Day 9 — Routing Strategies

- Nearest-latency, consistent hashing, multi-tier edge→regional→origin
- Pluggable interface with strategy selection via config

Day 10 — Workload & Catalog

- Zipfian popularity, time-series request generator, trace ingestion (optional)
- Deterministic seeds and reproducible runs

Day 11 — Origin Model

- Capacity/concurrency, latency distribution, throttling/backoff
- Integration with node fetch path

Day 12 — Consistency & Invalidation

- TTLs, versioned objects, purge/ban events, stale-while-revalidate
- Deterministic timing with event scheduling

Day 13 — Metrics & Reporting

- Per-request latency breakdown, hit/miss by tier, bandwidth, origin QPS
- Export CSV/JSON; summary generator and simple charts (optional)

Day 14 — Examples, CI, Perf

- Example configs: single PoP, multi-tier, failure-injection, policy comparison
- CI with tests, go bench; basic profiling and optimizations

Notes

- Keep runs deterministic via seed; enable reproducible sweeps
- Prefer composable interfaces for policies, routing, and workloads
- Start simple; add fidelity (bandwidth contention, queues, failures) iteratively
