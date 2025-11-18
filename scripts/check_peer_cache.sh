#!/usr/bin/env bash
set -euo pipefail

# Script to check what segments a peer has in its cache
# Usage: ./scripts/check_peer_cache.sh <peer_number>

PEER_NUM="${1:-}"
TRACKER_URL="${TRACKER_URL:-http://localhost:7070}"
REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"

if [[ -z "$PEER_NUM" ]]; then
  echo "Usage: $0 <peer_number>"
  echo "Example: $0 5"
  exit 1
fi

PEER_NAME="peer-${PEER_NUM}"

echo "=========================================="
echo "Peer Cache Check"
echo "=========================================="
echo "Peer: $PEER_NAME"
echo ""

# Check if peer container exists
if ! docker ps --format '{{.Names}}' | grep -Fxq "$PEER_NAME"; then
  echo "ERROR: Error: Peer container '$PEER_NAME' is not running"
  exit 1
fi

# Get peer info
echo " Step 1: Checking peer status..."
PEER_IP=$(docker inspect "$PEER_NAME" --format '{{range .NetworkSettings.Networks}}{{if .IPAddress}}{{printf "%s\n" .IPAddress}}{{end}}{{end}}' 2>/dev/null | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
PEER_NEIGHBORS=$(docker exec "$PEER_NAME" wget -qO- http://localhost:8080/peers 2>/dev/null || echo "")
echo "   Peer IP: $PEER_IP"
echo "   Neighbors: ${PEER_NEIGHBORS:-none}"
echo ""

# Query Redis for peer's segments (via tracker's data structure)
echo " Step 2: Querying tracker/Redis for cached segments..."

# Try to get segments from Redis directly
REDIS_SEGMENTS=$(docker exec cdn-redis-1 redis-cli SMEMBERS "peer:${PEER_NAME}:segments" 2>/dev/null || echo "")

if [[ -n "$REDIS_SEGMENTS" ]] && [[ "$REDIS_SEGMENTS" != "(empty array)" ]] && [[ "$REDIS_SEGMENTS" != "(empty list or set)" ]]; then
  SEGMENT_COUNT=$(echo "$REDIS_SEGMENTS" | grep -v "^$" | wc -l | tr -d ' ')
  if [[ -z "$SEGMENT_COUNT" ]] || [[ ! "$SEGMENT_COUNT" =~ ^[0-9]+$ ]]; then
    SEGMENT_COUNT=0
  fi
  
  if [[ "$SEGMENT_COUNT" -gt 0 ]]; then
    echo "   Found $SEGMENT_COUNT segment(s) in tracker registry:"
    echo ""
    while IFS= read -r seg; do
      if [[ -n "$seg" ]]; then
        echo "   OK: $seg"
      fi
    done <<< "$REDIS_SEGMENTS"
  else
    echo "   WARNING:  No segments found in tracker registry"
  fi
else
  echo "   WARNING:  No segments found in tracker registry"
  REDIS_SEGMENTS=""
fi
echo ""

# Verify segments by trying to fetch them from the peer
echo " Step 3: Checking peer's actual cache (scanning common segments)..."
COMMON_SEGMENTS=("segment000.ts" "segment001.ts" "segment002.ts" "segment003.ts" "segment004.ts" "segment005.ts" "segment006.ts" "segment007.ts")

# Build a set of segments from tracker
declare -A TRACKER_SEGMENTS
if [[ -n "$REDIS_SEGMENTS" ]] && [[ "$REDIS_SEGMENTS" != "(empty array)" ]] && [[ "$REDIS_SEGMENTS" != "(empty list or set)" ]]; then
  while IFS= read -r seg; do
    if [[ -n "$seg" ]]; then
      TRACKER_SEGMENTS["$seg"]=1
    fi
  done <<< "$REDIS_SEGMENTS"
fi

VERIFIED_COUNT=0
NOT_IN_TRACKER_COUNT=0
IN_TRACKER_BUT_MISSING=0

for seg in "${COMMON_SEGMENTS[@]}"; do
  # Try to fetch segment from peer
  FETCH_RESULT=$(timeout 2 docker exec "$PEER_NAME" wget -qO- --timeout=1 "http://localhost:8080/segments/${seg}" 2>/dev/null || echo "")
  
  if [[ -n "$FETCH_RESULT" ]] && [[ "$FETCH_RESULT" != "null" ]] && echo "$FETCH_RESULT" | grep -q '"id"'; then
    SEG_SIZE=$(echo "$FETCH_RESULT" | wc -c)
    if [[ -n "${TRACKER_SEGMENTS[$seg]:-}" ]]; then
      echo "   OK: $seg (${SEG_SIZE} bytes) - in tracker ✓"
      VERIFIED_COUNT=$((VERIFIED_COUNT + 1))
    else
      echo "   OK: $seg (${SEG_SIZE} bytes) - NOT in tracker (needs announcement)"
      NOT_IN_TRACKER_COUNT=$((NOT_IN_TRACKER_COUNT + 1))
    fi
  elif [[ -n "${TRACKER_SEGMENTS[$seg]:-}" ]]; then
    echo "   WARNING:  $seg (in tracker but not in peer cache)"
    IN_TRACKER_BUT_MISSING=$((IN_TRACKER_BUT_MISSING + 1))
  fi
done

echo ""
echo "   Summary:"
echo "   - In cache & tracker: $VERIFIED_COUNT"
echo "   - In cache but NOT in tracker: $NOT_IN_TRACKER_COUNT"
if [[ $IN_TRACKER_BUT_MISSING -gt 0 ]]; then
  echo "   - In tracker but missing from cache: $IN_TRACKER_BUT_MISSING"
fi
echo ""

# Get peer metadata from Redis if available
echo " Step 4: Peer metadata..."
PEER_META=$(docker exec cdn-redis-1 redis-cli GET "peer:${PEER_NAME}:meta" 2>/dev/null || echo "")
if [[ -n "$PEER_META" ]] && [[ "$PEER_META" != "(nil)" ]]; then
  PEER_REGION=$(echo "$PEER_META" | grep -o '"region":"[^"]*"' | cut -d'"' -f4 || echo "unknown")
  PEER_RTT=$(echo "$PEER_META" | grep -o '"rtt_ms":[0-9]*' | cut -d':' -f2 || echo "unknown")
  echo "   Region: ${PEER_REGION:-unknown}"
  echo "   RTT: ${PEER_RTT:-unknown}ms"
else
  echo "   WARNING:  No metadata found in tracker"
fi
echo ""

echo "=========================================="
echo "OK: Cache Check Complete"
echo "=========================================="

