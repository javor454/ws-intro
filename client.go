package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

var (
	pongWait     = 10 * time.Second    // wait max secs until connection dropped
	pingInterval = (pongWait * 9) / 10 // shorter than pong interval by 10%
)

// map key can be anything comparable = pointers to a struct
type ClientList map[*Client]bool

type Client struct {
	conn    *websocket.Conn
	manager *Manager

	chatroom string
	// used to avoid concurrent writes on the ws conn
	egress chan Event
}

func NewClient(conn *websocket.Conn, manager *Manager) *Client {
	return &Client{conn: conn, manager: manager, egress: make(chan Event)}
}

func (c *Client) readMessages() {
	fmt.Println("read messages init")
	defer func() {
		// cleanup
		c.manager.removeClient(c)
	}()

	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		log.Println("Error setting read deadline:", err)
		return
	}

	c.conn.SetPongHandler(c.pongHandler)

	// jumbo frames - restrict sending large messages
	c.conn.SetReadLimit(512)

	for {
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			fmt.Println("conn is closed")
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Printf("err reading messages: %v\n", err)
			}
			break
		}

		var request Event
		if err := json.Unmarshal(payload, &request); err != nil {
			fmt.Printf("err unmarshalling payload: %v\n", err)
			break
		}

		if err := c.manager.routeEvent(request, c); err != nil {
			fmt.Printf("err routeing event: %v\n", err)
		}
	}
}

func (c *Client) writeMessages() {
	defer func() {
		c.manager.removeClient(c)
	}()

	ticket := time.NewTicker(pingInterval)

	for {
		select {
		case message, ok := <-c.egress:
			if !ok {
				if err := c.conn.WriteMessage(websocket.CloseMessage, nil); err != nil {
					log.Println("connection closed: ", err)
				}
				return
			}

			data, err := json.Marshal(message)
			if err != nil {
				fmt.Println("err marshalling message: ", err)
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Println("failed to send message: ", err)
			}
		case <-ticket.C:
			log.Println("ping")
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Println("failed to send ping: ", err)
				return
			}
		}
	}
}

func mapMessageType(messageType int) string {
	switch messageType {
	case websocket.TextMessage:
		return "text"
	case websocket.BinaryMessage:
		return "binary"
	case websocket.CloseMessage:
		return "close"
	case websocket.PingMessage:
		return "ping"
	case websocket.PongMessage:
		return "pong"
	default:
		return "unknown"
	}
}

func (c *Client) pongHandler(pongMsg string) error {
	log.Println("pong")
	return c.conn.SetReadDeadline(time.Now().Add(pongWait))
}
