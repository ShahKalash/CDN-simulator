package main

import (
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type string                 `json:"type"`
	Room string                 `json:"room"`
	From string                 `json:"from"`
	To   string                 `json:"to"`
	Data map[string]interface{} `json:"data"`
}

type Client struct {
	id   string
	room string
	conn *websocket.Conn
	send chan Message
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client
	rooms   map[string]map[string]*Client
}

func NewHub() *Hub {
	return &Hub{clients: map[string]*Client{}, rooms: map[string]map[string]*Client{}}
}

func (h *Hub) Join(c *Client, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c.room = room
	if h.rooms[room] == nil {
		h.rooms[room] = map[string]*Client{}
	}
	h.rooms[room][c.id] = c
}

func (h *Hub) Leave(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r := h.rooms[c.room]; r != nil {
		delete(r, c.id)
	}
	delete(h.clients, c.id)
}

func (h *Hub) Broadcast(room string, m Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.rooms[room] {
		select {
		case c.send <- m:
		default:
		}
	}
}

func (h *Hub) Direct(to string, m Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if c := h.clients[to]; c != nil {
		select {
		case c.send <- m:
		default:
		}
	}
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func main() {
	addr := getenv("WS_ADDR", ":8091")
	hub := NewHub()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			id = time.Now().Format("150405.000000")
		}
		c := &Client{id: id, conn: conn, send: make(chan Message, 16)}

		hub.mu.Lock()
		hub.clients[id] = c
		hub.mu.Unlock()

		go func() { // writer
			for m := range c.send {
				_ = c.conn.WriteJSON(m)
			}
		}()

		for { // reader
			var msg Message
			if err := conn.ReadJSON(&msg); err != nil {
				break
			}
			switch msg.Type {
			case "join":
				hub.Join(c, msg.Room)
			case "offer", "answer", "ice":
				if msg.To != "" {
					hub.Direct(msg.To, msg)
				} else {
					hub.Broadcast(c.room, msg)
				}
			case "leave":
				hub.Leave(c)
				_ = conn.Close()
				return
			}
		}
		hub.Leave(c)
		_ = conn.Close()
	})

	log.Printf("signaling listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
