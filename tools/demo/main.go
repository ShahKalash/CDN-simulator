package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	tracker := getenv("TRACKER", "http://localhost:8090")
	signaling := getenv("SIGNALING", "ws://localhost:8091/ws")

	httpClient := &http.Client{Timeout: 5 * time.Second}

	mustStatus(httpClient, tracker+"/healthz", 200)
	log.Println("tracker healthy")

	seg := getenv("DEMO_SEG", "rickroll/128k/index0.m4s")
	// announce two peers
	announce(httpClient, tracker+"/announce", "peerA", seg, "us", 40)
	announce(httpClient, tracker+"/announce", "peerB", seg, "us", 60)
	log.Println("announced peerA and peerB")

	// lookup
	resp, err := httpClient.Get(fmt.Sprintf("%s/peers?seg=%s&count=5&region=us", tracker, seg))
	must(err)
	defer resp.Body.Close()
	var peers []map[string]any
	must(json.NewDecoder(resp.Body).Decode(&peers))
	log.Printf("peers lookup returned %d entries\n", len(peers))

	// signaling: connect two clients and exchange an offer
	room := fmt.Sprintf("room-%d", rand.Intn(100000))
	a := dialWS(signaling + "?id=a")
	b := dialWS(signaling + "?id=b")
	defer a.Close()
	defer b.Close()

	must(a.WriteJSON(map[string]any{"type": "join", "room": room}))
	must(b.WriteJSON(map[string]any{"type": "join", "room": room}))
	log.Println("both joined", room)

	// reader on B
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var msg map[string]any
			if err := b.ReadJSON(&msg); err != nil {
				log.Printf("b read err: %v\n", err)
				return
			}
			if msg["type"] == "offer" {
				log.Println("B received offer from A")
				return
			}
		}
	}()

	must(a.WriteJSON(map[string]any{"type": "offer", "room": room, "to": "b", "data": map[string]any{"sdp": "test-offer"}}))

	select {
	case <-done:
		log.Println("signaling message delivered")
	case <-ctx.Done():
		log.Fatal("timeout waiting for signaling message")
	}

	log.Println("demo completed successfully")
}

func mustStatus(c *http.Client, url string, want int) {
	resp, err := c.Get(url)
	must(err)
	defer resp.Body.Close()
	if resp.StatusCode != want {
		log.Fatalf("GET %s status=%d want=%d", url, resp.StatusCode, want)
	}
}

func announce(c *http.Client, url, peer, seg, region string, rtt int) {
	body := map[string]any{
		"peerId":   peer,
		"segments": []string{seg},
		"region":   region,
		"rtt":      rtt,
		"lastSeen": time.Now().Unix(),
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	must(err)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		log.Fatalf("announce status=%d", resp.StatusCode)
	}
}

func dialWS(url string) *websocket.Conn {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	must(err)
	return conn
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

