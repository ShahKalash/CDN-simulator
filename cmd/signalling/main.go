package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud_project/internal/signalling"
	"github.com/gorilla/websocket"
)

type inboundMessage struct {
	Type      string            `json:"type"`
	Peer      string            `json:"peer"`
	Room      string            `json:"room"`
	Target    string            `json:"target"`
	Neighbors []string          `json:"neighbors"`
	Metadata  map[string]any    `json:"metadata"`
	Payload   map[string]string `json:"payload"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	addr := env("SIGNAL_ADDR", ":7080")
	hub := signalling.NewHub()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebsocket(hub, w, r)
	})

	log.Printf("Signalling server listening on %s", addr)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func handleWebsocket(hub *signalling.Hub, w http.ResponseWriter, r *http.Request) {
	peer := r.URL.Query().Get("peer")
	room := r.URL.Query().Get("room")
	if peer == "" || room == "" {
		http.Error(w, "peer and room are required", http.StatusBadRequest)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	connection := signalling.NewConnection(signalling.PeerID(peer), conn)
	hub.Register(room, connection)
	defer hub.Unregister(room, signalling.PeerID(peer))

	go connection.WriteLoop(ctx)

	err = connection.ReadLoop(func(msg []byte) {
		var inbound inboundMessage
		if err := json.Unmarshal(msg, &inbound); err != nil {
			log.Printf("invalid message from %s: %v", peer, err)
			return
		}
		processMessage(ctx, hub, room, connection, inbound)
	})
	if err != nil {
		log.Printf("read loop ended for %s: %v", peer, err)
	}
}

func processMessage(ctx context.Context, hub *signalling.Hub, room string, conn *signalling.Connection, msg inboundMessage) {
	switch strings.ToLower(msg.Type) {
	case "announce":
		neighbors := make([]signalling.PeerID, 0, len(msg.Neighbors))
		for _, n := range msg.Neighbors {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			neighbors = append(neighbors, signalling.PeerID(n))
		}
		hub.Announce(room, signalling.Announcement{
			Peer:      signalling.PeerID(msg.Peer),
			Room:      room,
			Neighbors: neighbors,
			Metadata:  msg.Metadata,
		})
	case "request_path":
		target := signalling.PeerID(msg.Target)
		resp, err := hub.ShortestPath(room, conn.Peer, target)
		if err != nil {
			log.Printf("path error %s->%s: %v", conn.Peer, target, err)
			return
		}
		if err := hub.BroadcastPath(ctx, room, resp.Path); err != nil {
			log.Printf("broadcast path: %v", err)
		}
	default:
		log.Printf("unhandled message type %s from %s", msg.Type, conn.Peer)
	}
}

func env(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}

