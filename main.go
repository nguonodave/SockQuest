package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/gorilla/websocket"
)

type Message struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

type UserStatus struct {
	Username string `json:"username"`
	Status   string `json:"status"`
}

type Envelope struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
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

	createMessagesTable := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_user TEXT,
		to_user TEXT,
		content TEXT,
		timestamp TEXT,
		read INTEGER DEFAULT 0
	);`

	createUsersTable := `
	CREATE TABLE IF NOT EXISTS users (
		username TEXT PRIMARY KEY NOT NULL UNIQUE,
		password TEXT NOT NULL
	);`

	_, err = db.Exec(createMessagesTable)
	if err != nil {
		log.Fatal("Failed to create messages table:", err)
	}

	_, err = db.Exec(createUsersTable)
	if err != nil {
		log.Fatal("Failed to create users table:", err)
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

	broadcastUserList()

	// Clean up on disconnect
	defer func() {
		delete(users, username)
		log.Println(username, "disconnected")
		broadcastUserList()
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

		_, insertMessageErr := db.Exec(`
		INSERT INTO messages (from_user, to_user, content, timestamp)
		VALUES (?, ?, ?, ?)`, msg.From, msg.To, msg.Content, msg.Timestamp)
		if insertMessageErr != nil {
			log.Println("Failed to save message to database: ", insertMessageErr)
		}

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

		// Update the user list for both sender and recipient
		broadcastUserList()
	}
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	// Insert user into database
	stmt := `INSERT INTO users (username, password) VALUES (?, ?)`
	_, err = db.Exec(stmt, creds.Username, creds.Password)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "Username already exists", http.StatusConflict)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Registration successful",
	})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method allowed", http.StatusMethodNotAllowed)
		return
	}

	var logincreds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	jsonlogerr := json.NewDecoder(r.Body).Decode(&logincreds)
	if jsonlogerr != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	if logincreds.Username == "" || logincreds.Password == "" {
		http.Error(w, "Username and password required", http.StatusBadRequest)
		return
	}

	var storedPassword string
	err := db.QueryRow("SELECT password FROM users WHERE username = ?", logincreds.Username).Scan(&storedPassword)
	if err == sql.ErrNoRows {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	} else if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	if logincreds.Password != storedPassword {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	// Generate random session ID
	sessionID := make([]byte, 16)
	_, err = rand.Read(sessionID)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}
	sessionIDStr := hex.EncodeToString(sessionID)
	sessions[sessionIDStr] = logincreds.Username

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionIDStr,
		Path:     "/",
		HttpOnly: true,
		// Secure: true, // uncomment when using HTTPS
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Login successful",
	})
}

var sessions = make(map[string]string) // sessionID -> username

func handleSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"loggedIn": false}`, http.StatusUnauthorized)
		return
	}

	username := sessions[cookie.Value]
	if username == "" {
		http.Error(w, `{"loggedIn": false}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"loggedIn": true,
		"username": username,
	})
}

func handleUserStatuses(w http.ResponseWriter, r *http.Request) {
	currentUser := r.URL.Query().Get("currentUser")
	rows, err := db.Query("SELECT username FROM users WHERE username != ? ORDER BY username", currentUser)
	if err != nil {
		http.Error(w, "Failed to query users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type userWithTime struct {
		username string
		lastTime time.Time
	}

	var userStatuses []userWithTime
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			continue
		}

		// Get last message time
		timestamp, err := getLastMessageTimestamp(currentUser, username)
		var lastTime time.Time
		if err == nil && timestamp != "" {
			lastTime, _ = time.Parse(time.RFC3339, timestamp)
		}

		userStatuses = append(userStatuses, userWithTime{
			username: username,
			lastTime: lastTime,
		})
	}

	// Sort by last message time (newest first), then alphabetically
	sort.Slice(userStatuses, func(i, j int) bool {
		if !userStatuses[i].lastTime.IsZero() || !userStatuses[j].lastTime.IsZero() {
			if userStatuses[i].lastTime.Equal(userStatuses[j].lastTime) {
				return userStatuses[i].username < userStatuses[j].username
			}
			return userStatuses[i].lastTime.After(userStatuses[j].lastTime)
		}
		return userStatuses[i].username < userStatuses[j].username
	})

	// Convert to final output format
	var result []map[string]string
	for _, user := range userStatuses {
		status := "offline"
		if _, ok := users[user.username]; ok {
			status = "online"
		}
		result = append(result, map[string]string{
			"username": user.username,
			"status":   status,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// getLastMessageTimestamp gets the timestamp of the last message between user1 and user2
func getLastMessageTimestamp(user1, user2 string) (string, error) {
	var timestamp string
	err := db.QueryRow(`
        SELECT timestamp 
        FROM messages 
        WHERE (from_user = ? AND to_user = ?) OR (from_user = ? AND to_user = ?)
        ORDER BY timestamp DESC
        LIMIT 1`,
		user1, user2, user2, user1).Scan(&timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return timestamp, nil
}

func broadcastUserList() {
	// Get all users except the current user (we'll need to know who's requesting)
	// Since this is broadcast, we'll need to handle this differently
	// We'll need to send a personalized list to each connected user

	for username, conn := range users {
		// For each connected user, get their personalized sorted list
		rows, err := db.Query("SELECT username FROM users WHERE username != ?", username)
		if err != nil {
			log.Println("Failed to query users:", err)
			continue
		}

		type userWithTime struct {
			username string
			lastTime time.Time
		}

		var userStatuses []userWithTime
		for rows.Next() {
			var otherUser string
			if err := rows.Scan(&otherUser); err != nil {
				continue
			}

			// Get last message time between current user and this other user
			timestamp, err := getLastMessageTimestamp(username, otherUser)
			var lastTime time.Time
			if err == nil && timestamp != "" {
				lastTime, _ = time.Parse(time.RFC3339, timestamp)
			}

			userStatuses = append(userStatuses, userWithTime{
				username: otherUser,
				lastTime: lastTime,
			})
		}
		rows.Close()

		// Sort by last message time (newest first), then alphabetically
		sort.Slice(userStatuses, func(i, j int) bool {
			if !userStatuses[i].lastTime.IsZero() || !userStatuses[j].lastTime.IsZero() {
				if userStatuses[i].lastTime.Equal(userStatuses[j].lastTime) {
					return userStatuses[i].username < userStatuses[j].username
				}
				return userStatuses[i].lastTime.After(userStatuses[j].lastTime)
			}
			return userStatuses[i].username < userStatuses[j].username
		})

		// Convert to final output format
		var result []UserStatus
		for _, user := range userStatuses {
			status := "offline"
			if _, ok := users[user.username]; ok {
				status = "online"
			}
			result = append(result, UserStatus{
				Username: user.username,
				Status:   status,
			})
		}

		payload := Envelope{
			Type: "userlist",
			Data: result,
		}

		err = conn.WriteJSON(payload)
		if err != nil {
			log.Println("Failed to send user list update:", err)
			conn.Close()
			delete(users, username)
		}
	}
}

// Add this to your main.go
func handleConversationHistory(w http.ResponseWriter, r *http.Request) {
	currentUser := r.URL.Query().Get("currentUser")
	selectedUser := r.URL.Query().Get("selectedUser")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	if currentUser == "" || selectedUser == "" {
		http.Error(w, "Both users must be specified", http.StatusBadRequest)
		return
	}

	// Default values
	limit := 10
	offset := 0

	// Parse limit if provided
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit < 0 {
			http.Error(w, "Invalid limit value", http.StatusBadRequest)
			return
		}
	}

	// Parse offset if provided
	if offsetStr != "" {
		var err error
		offset, err = strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			http.Error(w, "Invalid offset value", http.StatusBadRequest)
			return
		}
	}

	query := `
        SELECT from_user, to_user, content, timestamp 
        FROM messages 
        WHERE (from_user = ? AND to_user = ?)
           OR (from_user = ? AND to_user = ?)
        ORDER BY timestamp DESC
		LIMIT ? OFFSET ?`

	rows, err := db.Query(query, currentUser, selectedUser, selectedUser, currentUser, limit, offset)
	if err != nil {
		log.Println(err)
		http.Error(w, "Failed to query messages", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.From, &msg.To, &msg.Content, &msg.Timestamp); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	// Since we ordered DESC for newest first, reverse them before sending
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func countUnreadMessages(username string) (map[string]int, error) {
    query := `
        SELECT from_user, COUNT(*) as count 
        FROM messages 
        WHERE to_user = ? AND read = 0
        GROUP BY from_user`
    
    rows, err := db.Query(query, username)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    unreadCounts := make(map[string]int)
    for rows.Next() {
        var fromUser string
        var count int
        if err := rows.Scan(&fromUser, &count); err != nil {
            continue
        }
        unreadCounts[fromUser] = count
    }
    return unreadCounts, nil
}

func handleMarkAsRead(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    var data struct {
        CurrentUser string `json:"currentUser"`
        FromUser    string `json:"fromUser"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
        http.Error(w, "Invalid input", http.StatusBadRequest)
        return
    }

    _, err := db.Exec(`
        UPDATE messages 
        SET read = 1 
        WHERE from_user = ? AND to_user = ? AND read = 0`,
        data.FromUser, data.CurrentUser)
    
    if err != nil {
        http.Error(w, "Failed to update messages", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}

func handleUnreadCounts(w http.ResponseWriter, r *http.Request) {
    username := r.URL.Query().Get("username")
    if username == "" {
        http.Error(w, "Username is required", http.StatusBadRequest)
        return
    }

    counts, err := countUnreadMessages(username)
    if err != nil {
        http.Error(w, "Failed to get unread counts", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(counts)
}

func main() {
	setupDatabase()

	// serve static files
	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/session", handleSession)
	http.HandleFunc("/users", handleUserStatuses)
	http.HandleFunc("/conversation", handleConversationHistory)
	http.HandleFunc("/unreadCounts", handleUnreadCounts)
	http.HandleFunc("/markAsRead", handleMarkAsRead)
	// WebSocket endpoint
	http.HandleFunc("/ws", handleWebSocket)

	go handleMessages()

	fmt.Println("Server started at http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
