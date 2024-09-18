package main

import (
	"io"
	"net/http"
	"orders-broadcaster/clients"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

var upgrader = websocket.Upgrader{}

func init() {
	log.SetFormatter(&log.JSONFormatter{})
}

// broadcaster handles sending a []byte channel message to all websocket clients
func broadcaster(activeClients *clients.Clients, message <-chan []byte) {
	for {
		m := <-message
		for host, client := range activeClients.GetAllClients() {
			err := client.WriteMessage(websocket.TextMessage, m)
			if err != nil {
				log.WithFields(log.Fields{
					"host":  client.RemoteAddr().String(),
					"error": err,
				}).Info("failed to send message to client, closing connection...")

				activeClients.RemoveClient(host)
			}
		}
	}
}

// poller requests a payload at a specific interval and pushed that payload to a message channel
func poller(pollingIntervals int, message chan<- []byte) {
	for {
		resp, err := http.Get("http://ip.jsontest.com")
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("failed pooling request")
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("failed pooling request")
		}

		message <- body

		time.Sleep(time.Millisecond * time.Duration(pollingIntervals))
	}
}

func handleConnect(clients chan<- *websocket.Conn) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.WithFields(log.Fields{
			"host": r.RemoteAddr,
		}).Info("new connection request")

		// enable all origins
		// TODO: Accept only valid origins
		upgrader.CheckOrigin = func(r *http.Request) bool { return true }
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.WithFields(log.Fields{
				"host": r.Host,
			}).Error("upgrade:", err)
			return
		}
		clients <- c
	}
}

func main() {
	// defaults
	port := "8080"
	pollingIntervalMs := 1000

	// number of orders
	conns := make(chan *websocket.Conn)
	messages := make(chan []byte)
	activeClients := clients.NewClient()

	go func() {
		http.HandleFunc("/connect", handleConnect(conns))
		log.WithFields(log.Fields{
			"port": port,
		}).Info("starting websocket server...")

		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatal(err)
		}
	}()

	// add any new connected clients to our client pool
	go func(conns <-chan *websocket.Conn, c *clients.Clients) {
		for {
			o := <-conns
			c.AddClient(o)
		}
	}(conns, &activeClients)

	// start the broadcast
	go broadcaster(&activeClients, messages)

	// start the poller
	go poller(pollingIntervalMs, messages)

	// List for SIGTERM and graceful shutdown, using buffer channel with len equal to number of signals.
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	os.Exit(1)
}
