package main

import (
	"fmt"
	"log"

	"github.com/gorilla/websocket"
)

// map key can be anything comparable = pointers to a struct
type ClientList map[*Client]bool

type Client struct {
	conn    *websocket.Conn
	manager *Manager

	// used to avoid concurrent writes on the ws conn
	egress chan []byte
}

func NewClient(conn *websocket.Conn, manager *Manager) *Client {
	return &Client{conn: conn, manager: manager, egress: make(chan []byte)}
}

func (c *Client) readMessages() {
	fmt.Println("read messages init")
	defer func() {
		// cleanup
		c.manager.removeClient(c)
	}()

	for {
		messageType, payload, err := c.conn.ReadMessage()
		if err != nil {
			fmt.Println("conn is closed")
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Printf("err reading messages: %v\n", err)
			}
			break
		}

		for wsclient := range c.manager.clients {
			wsclient.egress <- payload
		}

		fmt.Printf("message type: %s\n", mapMessageType(messageType))
		fmt.Println("payload:")
		fmt.Println(string(payload))
	}
}

func (c *Client) writeMessages() {
	defer func() {
		c.manager.removeClient(c)
	}()

	for {
		select {
		case message, ok := <-c.egress:
			if !ok {
				if err := c.conn.WriteMessage(websocket.CloseMessage, nil); err != nil {
					log.Println("connection closed: ", err)
				}
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Println("failed to send message: ", err)
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
