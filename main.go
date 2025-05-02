package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"

	"github.com/gorilla/websocket"
)

type Message struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

var (
	users     = make(map[string]*websocket.Conn)
	broadcast = make(chan Message)
	db        *sql.DB
)

// upgrader upgrades http conns to websocket conns
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all domain conns
	},
}

func setupDatabase() {
	var err error
	db, err = sql.Open("sqlite3", "./chat.db")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	createTable := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_user TEXT,
		to_user TEXT,
		content TEXT,
		timestamp TEXT
	);
	`

	_, err = db.Exec(createTable)
	if err != nil {
		log.Fatal("Failed to create messages table:", err)
	}

	log.Println("Database initialized successfully.")
}

// handleWebSocket uses the upgrader to upgrade the http conn
// then reads and writes messages from and to the ws
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// upgrade initial get request to a WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade failed:", err)
		return
	}
	defer conn.Close()

	fmt.Println("Client connected via WebSocket!")

	var loginMsg Message
	jsonErr := conn.ReadJSON(&loginMsg)
	if jsonErr != nil {
		log.Println("Login message read error:", err)
		return
	}

	username := loginMsg.From
	users[username] = conn
	log.Println(username, "connected")

	// Clean up on disconnect
	defer func() {
		delete(users, username)
		log.Println(username, "disconnected")
	}()

	for {
		// read message from user
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			fmt.Println("Read error from "+username+": ", err)
			break
		}

		broadcast <- msg
	}
}

func handleMessages() {
	for {
		msg := <-broadcast

		recipientConn, ok := users[msg.To]
		if ok {
			err := recipientConn.WriteJSON(msg)
			if err != nil {
				log.Println("Error sending message to", msg.To+":", err)
				recipientConn.Close()
				delete(users, msg.To)
			}
		} else {
			log.Println("User", msg.To, "is not online. Message not delivered.")
			// Later: store in DB for delivery when they return
		}
	}
}

func main() {
	setupDatabase()

	// serve static files
	http.Handle("/", http.FileServer(http.Dir("./static")))

	// WebSocket endpoint
	http.HandleFunc("/ws", handleWebSocket)

	go handleMessages()

	fmt.Println("Server started at http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
