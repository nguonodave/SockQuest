package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var clients = make(map[*websocket.Conn]bool)
var broadcast = make(chan []byte)

// upgrader upgrades http conns to websocket conns
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all domain conns
	},
}

// handleWebSocket uses the upgrader to upgrade the http conn
// then reads and writes messages from and to the ws
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// upgrade initial get request to a WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("WebSocket upgrade failed:", err)
		return
	}
	defer conn.Close()

	fmt.Println("Client connected via WebSocket!")

	clients[conn] = true

	for {
		// read message from client (browser)
		_, msg, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Read error:", err)
			delete(clients, conn)
			break
		}
		fmt.Println("Received message:", string(msg))

		broadcast <- msg

		// // send message to client (browser)
		// writeErr := conn.WriteMessage(websocket.TextMessage, msg)
		// if writeErr != nil {
		// 	fmt.Println("Error sending message:", err)
		// 	break
		// }
	}
}

func handleMessages()  {
	for {
		msg := <-broadcast

		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Println("Write error:", err)
                client.Close()
                delete(clients, client)
			}
		}
	}
}

func main() {
	// serve static files
	http.Handle("/", http.FileServer(http.Dir("./static")))

	// WebSocket endpoint
	http.HandleFunc("/ws", handleWebSocket)

	go handleMessages()

	fmt.Println("Server started at http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
