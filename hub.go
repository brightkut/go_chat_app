package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber/contrib/websocket"
)

// Inside hub.go (below the Client struct definition)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

// Read messages from the client's WebSocket connection and pass them to the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c // Signal hub to unregister this client on exit
		c.conn.Close()        // Close the actual WebSocket connection
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait)) // Set initial read deadline for pong
	c.conn.SetPongHandler(func(string) error {       // Reset read deadline when a pong is received
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// If it's a normal closure error, just log and break.
			// Otherwise, log the specific error.
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("read error for client %s: %v", c.id, err)
			} else {
				log.Printf("client %s disconnected normally or due to error: %v", c.id, err)
			}
			break // Exit the loop, defer will handle unregister/close
		}
		chatMsg := ChatMessage{
			UserID:    c.id,               // Use the client's ID
			Username:  "User " + c.id[:4], // Simple placeholder username for now
			Content:   string(message),
			Timestamp: time.Now(),
		}

		jsonMsg, err := json.Marshal(chatMsg) // Marshal the struct to JSON bytes
		if err != nil {
			log.Printf("Error marshalling chat message from client %s: %v", c.id, err)
			return // Or handle more gracefully
		}
		c.hub.broadcast <- jsonMsg
	}
}

// Write messages from the client's send channel to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod) // Create a ticker for sending pings
	defer func() {
		ticker.Stop()  // Stop the ticker on exit
		c.conn.Close() // Close the WebSocket connection
	}()

	for {
		select {
		case message, ok := <-c.send: // Try to read message from client's send channel
			c.conn.SetWriteDeadline(time.Now().Add(writeWait)) // Set write deadline
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{}) // Send close message
				return                                                // Exit writePump
			}

			w, err := c.conn.NextWriter(websocket.TextMessage) // Get a writer for the WebSocket
			if err != nil {
				log.Printf("Error getting writer for client %s: %v", c.id, err)
				return // Exit writePump on error
			}
			w.Write(message) // Write the message

			// Add queued chat messages to the current WebSocket message.
			// This optimizes by sending multiple messages in a single WebSocket frame
			// if they arrive quickly.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'}) // Separator for multiple messages
				w.Write(<-c.send)     // Write additional messages
			}

			if err := w.Close(); err != nil {
				log.Printf("Error closing writer for client %s: %v", c.id, err)
				return // Exit writePump on error
			}
		case <-ticker.C: // Ping period has elapsed
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Ping error for client %s: %v", c.id, err)
				return // Exit writePump on error
			}
		}
	}
}

// ChatMessage defines the structure of a chat message for JSON encoding/decoding.
type ChatMessage struct {
	UserID    string    `json:"userId"`    // The ID of the user sending the message
	Username  string    `json:"username"`  // The display name of the user
	Content   string    `json:"content"`   // The actual message text
	Timestamp time.Time `json:"timestamp"` // When the message was sent
}

type Hub struct {
	// register clients
	clients map[*Client]bool
	// channel for send message to all client
	broadcast chan []byte
	// register request from clients
	register chan *Client
	// unregister request from clients
	unregister chan *Client
}

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
	id   string
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			log.Println("Client registered. Total clients: ", len(h.clients))

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Println("Client unregistered. Total clients: ", len(h.clients))
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}

		}
	}
}
