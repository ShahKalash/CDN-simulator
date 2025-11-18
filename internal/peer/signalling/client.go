package signalling

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

type Client struct {
	url       string
	room      string
	peerID    string
	neighbors []string
	conn      *websocket.Conn
}

type announceMessage struct {
	Type      string   `json:"type"`
	Peer      string   `json:"peer"`
	Room      string   `json:"room"`
	Neighbors []string `json:"neighbors"`
}

type requestPathMessage struct {
	Type   string `json:"type"`
	Target string `json:"target"`
}

func NewClient(baseURL, room, peer string, neighbors []string) *Client {
	return &Client{
		url:       strings.TrimSuffix(baseURL, "/"),
		room:      room,
		peerID:    peer,
		neighbors: neighbors,
	}
}

func (c *Client) Connect(ctx context.Context) error {
	if c.url == "" {
		return nil
	}
	parsed, err := url.Parse(c.url)
	if err != nil {
		return err
	}
	query := parsed.Query()
	query.Set("peer", c.peerID)
	query.Set("room", c.room)
	parsed.RawQuery = query.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(parsed.String(), nil)
	if err != nil {
		return err
	}
	c.conn = conn
	go c.readLoop(ctx)
	return c.sendAnnounce()
}

func (c *Client) Close() {
	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage, nil)
		c.conn.Close()
	}
}

func (c *Client) sendAnnounce() error {
	if c.conn == nil {
		return nil
	}
	payload, _ := json.Marshal(announceMessage{
		Type:      "announce",
		Peer:      c.peerID,
		Room:      c.room,
		Neighbors: c.neighbors,
	})
	return c.conn.WriteMessage(websocket.TextMessage, payload)
}

func (c *Client) readLoop(ctx context.Context) {
	defer c.conn.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, msg, err := c.conn.ReadMessage()
			if err != nil {
				log.Printf("[peer %s] signalling closed: %v", c.peerID, err)
				return
			}
			log.Printf("[peer %s] signalling message: %s", c.peerID, string(msg))
		}
	}
}

func (c *Client) RequestPath(target string) error {
	if c.conn == nil {
		return nil
	}
	payload, _ := json.Marshal(requestPathMessage{
		Type:   "request_path",
		Target: target,
	})
	return c.conn.WriteMessage(websocket.TextMessage, payload)
}
