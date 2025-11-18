#!/usr/bin/env bash
set -euo pipefail

# Script to simulate a peer requesting music/segments
# Usage: ./scripts/simulate_peer_request.sh <peer_number> [segment_id]

PEER_NUM="${1:-}"
SEGMENT_ID="${2:-segment000.ts}"
TRACKER_URL="${TRACKER_URL:-http://localhost:7070}"
TOPOLOGY_URL="${TOPOLOGY_URL:-http://localhost:8090}"

if [[ -z "$PEER_NUM" ]]; then
  echo "Usage: $0 <peer_number> [segment_id]"
  echo "Example: $0 5 segment000.ts"
  exit 1
fi

PEER_NAME="peer-${PEER_NUM}"

echo "=========================================="
echo "🎵 Music Request Simulation"
echo "=========================================="
echo "Peer: $PEER_NAME"
echo "Segment: $SEGMENT_ID"
echo ""

# Check if peer container exists
if ! docker ps --format '{{.Names}}' | grep -Fxq "$PEER_NAME"; then
  echo "❌ Error: Peer container '$PEER_NAME' is not running"
  exit 1
fi

# Get peer info
echo "📡 Step 1: Checking peer status..."
PEER_IP=$(docker inspect "$PEER_NAME" --format '{{range .NetworkSettings.Networks}}{{if .IPAddress}}{{printf "%s\n" .IPAddress}}{{end}}{{end}}' 2>/dev/null | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
PEER_NEIGHBORS=$(docker exec "$PEER_NAME" wget -qO- http://localhost:8080/peers 2>/dev/null || echo "")
echo "   Peer IP: $PEER_IP"
echo "   Neighbors: ${PEER_NEIGHBORS:-none}"
echo ""

# Query tracker for available peers with this segment
echo "🔍 Step 2: Querying tracker for segment '$SEGMENT_ID'..."
TRACKER_RESPONSE=$(curl -s "${TRACKER_URL}/segments/${SEGMENT_ID}?region=global" 2>/dev/null || echo '{"segment":"'$SEGMENT_ID'","peers":[]}')

# Check if peers array is empty
if echo "$TRACKER_RESPONSE" | grep -q '"peers":\[\]'; then
  PEER_COUNT=0
else
  PEER_COUNT=$(echo "$TRACKER_RESPONSE" | grep -o '"peer_id"' | wc -l)
  if [[ -z "$PEER_COUNT" ]]; then
    PEER_COUNT=0
  fi
fi
echo "   Found $PEER_COUNT peer(s) with segment '$SEGMENT_ID'"
echo ""

if [[ "$PEER_COUNT" -eq 0 ]]; then
  echo "⚠️  No peers found with segment '$SEGMENT_ID'"
  echo ""
  echo "📊 Routing Decision:"
  echo "   Source: ORIGIN (no peers available)"
  echo "   Path: $PEER_NAME → edge-server → origin-server"
  echo "   Estimated Latency: ~150ms"
  echo ""
  echo "💡 To populate segments, peers need to announce them via:"
  echo "   curl -X POST http://tracker:7070/announce -H 'Content-Type: application/json' -d '{...}'"
  exit 0
fi

# Parse tracker response to find best peer
BEST_PEER=$(echo "$TRACKER_RESPONSE" | grep -o '"peer_id":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")
BEST_RTT=$(echo "$TRACKER_RESPONSE" | grep -o '"rtt_ms":[0-9]*' | head -1 | cut -d':' -f2 || echo "50")
BEST_REGION=$(echo "$TRACKER_RESPONSE" | grep -o '"region":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "unknown")

if [[ -z "$BEST_PEER" ]]; then
  echo "❌ Error: Could not parse tracker response"
  echo "Response: $TRACKER_RESPONSE"
  exit 1
fi

echo "✅ Best peer found: $BEST_PEER"
echo "   Region: $BEST_REGION"
echo "   RTT: ${BEST_RTT}ms"
echo ""

# Check if best peer is a direct neighbor
IS_NEIGHBOR="false"
if echo "$PEER_NEIGHBORS" | grep -q "$BEST_PEER"; then
  IS_NEIGHBOR="true"
fi

# Find path through topology
echo "🗺️  Step 3: Finding routing path..."
ROUTE_RESPONSE=$(curl -s "${TOPOLOGY_URL}/path?from=${PEER_NAME}&to=${BEST_PEER}" || echo '{"path":[]}')

# Extract path from response
ROUTING_PATH=$(echo "$ROUTE_RESPONSE" | grep -o '\[[^]]*\]' | head -1 || echo "[]")
HOP_COUNT=$(echo "$ROUTING_PATH" | grep -o ',' | wc -l || echo "0")

if [[ "$IS_NEIGHBOR" == "true" ]]; then
  HOP_COUNT=1
  ROUTING_PATH="[\"${PEER_NAME}\",\"${BEST_PEER}\"]"
fi

echo "   Path: $ROUTING_PATH"
echo "   Hops: $HOP_COUNT"
echo ""

# Routing decision
echo "📊 Step 4: Routing Decision"
if [[ "$IS_NEIGHBOR" == "true" ]]; then
  echo "   ✅ Source: P2P (Direct Neighbor)"
  echo "   Path: $PEER_NAME → $BEST_PEER"
  echo "   Latency: ~${BEST_RTT}ms (1 hop)"
elif [[ "$HOP_COUNT" -le 3 && "$BEST_RTT" -lt 100 ]]; then
  echo "   ✅ Source: P2P (Multi-hop)"
  echo "   Path: $ROUTING_PATH"
  echo "   Latency: ~$((BEST_RTT + HOP_COUNT * 10))ms ($HOP_COUNT hops)"
else
  echo "   ⚠️  Source: EDGE (P2P too slow/distant)"
  echo "   Path: $PEER_NAME → edge-server"
  echo "   Latency: ~80ms"
fi
echo ""

# Try to fetch segment from best peer
echo "📥 Step 5: Attempting to fetch segment..."
BEST_PEER_IP=$(docker inspect "$BEST_PEER" --format '{{range .NetworkSettings.Networks}}{{if .IPAddress}}{{printf "%s\n" .IPAddress}}{{end}}{{end}}' 2>/dev/null | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "")

if [[ -n "$BEST_PEER_IP" ]]; then
  FETCH_RESPONSE=$(docker exec "$PEER_NAME" wget -qO- "http://${BEST_PEER_IP}:8080/segments/${SEGMENT_ID}" 2>/dev/null || echo "")
  
  if [[ -n "$FETCH_RESPONSE" ]]; then
    echo "   ✅ Successfully fetched segment from $BEST_PEER"
    echo "   Segment size: $(echo "$FETCH_RESPONSE" | wc -c) bytes"
  else
    echo "   ⚠️  Segment not found in $BEST_PEER's cache (may need to be uploaded first)"
  fi
else
  echo "   ⚠️  Could not determine IP for $BEST_PEER"
fi
echo ""

echo "=========================================="
echo "✅ Simulation Complete"
echo "=========================================="

