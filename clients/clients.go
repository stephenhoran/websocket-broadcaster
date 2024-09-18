package clients

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/gorilla/websocket"
)

type Clients struct {
	m             sync.Mutex
	ActiveClients map[string]*websocket.Conn
}

// addClient adds a new active client. This is concurrent safe.
func (c *Clients) AddClient(conn *websocket.Conn) {
	c.m.Lock()
	defer c.m.Unlock()
	host := conn.RemoteAddr().String()

	go c.watch(host) // we start a watch routine to see if the client gracefully closes requests

	c.ActiveClients[host] = conn
}

// RemoveClient attempts to close an outstanding connection and remove the client from the pool
func (c *Clients) RemoveClient(host string) {
	c.m.Lock()
	defer c.m.Unlock()

	client := c.GetClient(host)
	client.Close() // we don't handle connection close errors. The client already unreachable, lets just drop it from the pool

	delete(c.ActiveClients, host)
}

// GetAllClients return the map of all connected clients
func (c *Clients) GetAllClients() map[string]*websocket.Conn {
	return c.ActiveClients
}

// GetClient returns a specific websocket connection
func (c *Clients) GetClient(host string) *websocket.Conn {
	return c.ActiveClients[host]
}

// watch handles a single client that request a close connection and removes them from the pool safely
func (c *Clients) watch(host string) {
	client := c.GetClient(host)
	for {
		mt, _, err := client.ReadMessage()
		if err != nil {
			log.WithFields(log.Fields{
				"host":  host,
				"error": err,
			}).Info("failed to read message from client, closing connection...")

			c.RemoveClient(host)
			break
		}

		if mt == websocket.CloseMessage {
			log.WithFields(log.Fields{
				"host":   host,
				"action": "close",
			}).Info("recieved close connection request")

			c.RemoveClient(host)
			break
		}
	}
}

func NewClient() Clients {
	c := Clients{ActiveClients: make(map[string]*websocket.Conn)}

	return c
}
