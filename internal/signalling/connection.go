package signalling

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type Connection struct {
	Peer PeerID
	Conn *websocket.Conn
	Send chan []byte
}

func NewConnection(peer PeerID, conn *websocket.Conn) *Connection {
	c := &Connection{
		Peer: peer,
		Conn: conn,
		Send: make(chan []byte, 32),
	}
	conn.SetReadLimit(64 * 1024)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	return c
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 50 * time.Second
	maxMessageSize = 64 * 1024
)

func (c *Connection) ReadLoop(handle func([]byte)) error {
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			return err
		}
		handle(message)
	}
}

func (c *Connection) WriteLoop(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("write error to %s: %v", c.Peer, err)
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

type outboundPath struct {
	Type string   `json:"type"`
	Path []PeerID `json:"path"`
}

func (c *Connection) SendPath(ctx context.Context, path []PeerID) {
	payload, err := json.Marshal(outboundPath{
		Type: "path",
		Path: path,
	})
	if err != nil {
		log.Printf("marshal path: %v", err)
		return
	}
	select {
	case c.Send <- payload:
	case <-ctx.Done():
	}
}

