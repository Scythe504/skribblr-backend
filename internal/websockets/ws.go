package websockets

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/scythe504/skribblr-backend/internal"
	"github.com/scythe504/skribblr-backend/internal/utils"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	rooms   = make(map[string]*internal.Room)
	roomsMu sync.RWMutex
)

func handleGuess(player *internal.Player, data interface{}) {
	guess, ok := data.(string)
	if !ok {
		log.Printf("Invalid guess data type: %T", data)
		return
	}

	log.Printf("=== Processing Guess ===")
	log.Printf("Player: %s", player.Username)
	log.Printf("Guess: %s", guess)

	room := player.Room
	// First, read the word with a read lock
	room.Mu.RLock()
	currentWord := room.Word
	room.Mu.RUnlock()

	// Check if guess is correct
	if guess == currentWord {
		log.Printf("Correct guess!")

		// Now lock for modifications
		room.Mu.Lock()
		player.Score += 100
		currentScore := player.Score
		if room.Current != nil {
			room.Current.Score += 50
		}
		room.Mu.Unlock()

		// Prepare broadcast message without holding the lock
		msg := internal.Message[any]{
			Type: "correct_guess",
			Data: map[string]any{
				"player": player.Username,
				"word":   currentWord,
				"score":  currentScore,
			},
		}

		log.Printf("Broadcasting correct guess message: %+v", msg)
		// Broadcast should handle its own locking
		broadcastToRoom(room, msg)

		// Start new round after broadcast
		go startNewRound(room)
	} else {
		log.Printf("Incorrect guess")
	}
	log.Printf("=== End Processing Guess ===")
}

func broadcastToRoom(room *internal.Room, msg internal.Message[any]) {
	log.Printf("=== Start Broadcast Debug ===")

	// Get a snapshot of players with read lock
	room.Mu.RLock()
	players := make(map[string]*internal.Player)
	for id, player := range room.Players {
		players[id] = player
	}
	room.Mu.RUnlock()

	log.Printf("Room ID: %s", room.Id)
	log.Printf("Message to broadcast: %+v", msg)
	log.Printf("Number of players in room: %d", len(players))

	for id, player := range players {
		log.Printf("Broadcasting to player ID: %s", id)
		if player == nil {
			log.Printf("Player is nil for ID: %s, skipping", id)
			continue
		}

		if player.Conn == nil {
			log.Printf("Connection is nil for player ID: %s, skipping", id)
			continue
		}

		err := player.Conn.WriteJSON(msg)
		if err != nil {
			log.Printf("Failed to send to player %s: %v", id, err)
			// Handle disconnected players
			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				room.Mu.Lock()
				delete(room.Players, id)
				room.Mu.Unlock()
			}
		} else {
			log.Printf("Successfully sent message to player %s", id)
		}
	}
	log.Printf("=== End Broadcast Debug ===")
}

func startNewRound(room *internal.Room) {
	log.Printf("=== Starting New Round ===")

	room.Mu.Lock()
	// Generate new word here - replace with your word generation logic
	room.Word = "NewWord"
	newWord := room.Word
	room.Mu.Unlock()

	msg := internal.Message[any]{
		Type: "new_round",
		Data: map[string]any{
			"message": "New round has started",
			"word":    newWord,
		},
	}

	log.Printf("Broadcasting new round message")
	broadcastToRoom(room, msg)
}

// Also modify HandleWebSocket to properly add the player
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade failed: ", err)
		return
	}

	username := r.URL.Query().Get("username")

	if username == "" {
		username = "Anonymous"
	}
	player := &internal.Player{
		Id:       utils.GenerateID(8),
		Conn:     conn,
		Username: username,
		Score:    0,
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 2 {
		conn.Close()
		return
	}

	roomId := parts[len(parts)-1]

	if err := AddPlayer(roomId, player); err != nil {
		log.Printf("Failed to add player to room: %v", err)
		conn.Close()
		return
	}

	go handleMessages(player)
}

func GetJoinableRoom() string {
	roomsMu.RLock()
	defer roomsMu.RUnlock()

	for _, room := range rooms {
		room.Mu.RLock()
		playerCount := len(room.Players)
		room.Mu.RUnlock()
		if playerCount < 6 {
			return room.Id
		}
	}

	return ""
}

func getOrCreateRoom(roomId string) *internal.Room {
	roomsMu.Lock()
	defer roomsMu.Unlock()

	room, exists := rooms[roomId]
	if !exists {
		room = &internal.Room{
			Id:      roomId,
			Players: make(map[string]*internal.Player),
			Word:    "Echo",
			Mu:      sync.RWMutex{}, // Initialize the mutex
		}
		rooms[roomId] = room
	}
	return room
}

func handleMessages(player *internal.Player) {
	defer func() {
		player.Conn.Close()	
		removePlayer(player)
	}()

	log.Printf("Started message handler for player: %s in room: %s", player.Username, player.Room.Id)

	for {
		_, rawMessage, err := player.Conn.ReadMessage()
		if err != nil {
			log.Printf("Read error occured during websocket message %s, %v", player.Username, err)
			break
		}

		var baseMsg internal.Message[json.RawMessage]

		if err := json.Unmarshal(rawMessage, &baseMsg); err != nil {
			log.Printf("Failed to parse base message: %v", err)
			continue
		}

		log.Printf("Received message type: %s from player: %s", baseMsg.Type, player.Username)

		switch baseMsg.Type {
		case "pixel_draw":
			handlePixelDraw(player, baseMsg.Data)
		case "guess":
			var guess string
			if err := json.Unmarshal(baseMsg.Data, &guess); err != nil {
				log.Printf("Failed to parse guess data: %v", err)
				continue
			}
			handleGuess(player, guess)	
		default:
			log.Printf("Unknown message type: %s", baseMsg.Type)
		}
	}
}

func broadcastToRoomExcept(room *internal.Room, msg internal.Message[any], excludePlayer *internal.Player) {
	room.Mu.RLock()
	defer room.Mu.RUnlock()

	for _, player := range room.Players {
		if player != excludePlayer {
			if err := player.Conn.WriteJSON(msg); err != nil {
				log.Printf("Failed to send message to player %s: %v", player.Id, err)
			}
		}
	}
}

func removePlayer(player *internal.Player) {
	room := player.Room

	room.Mu.Lock()
	delete(room.Players, player.Id)
	isEmpty := len(room.Players) == 0
	room.Mu.Unlock()

	if isEmpty {
		roomsMu.Lock()
		delete(rooms, room.Id)
		roomsMu.Unlock()
	}

	msg := internal.Message[any]{
		Type: "player_left",
		Data: player.Username,
	}

	broadcastToRoom(room, msg)
}

func AddPlayer(roomId string, player *internal.Player) error {
	room := getOrCreateRoom(roomId)
	player.Room = room
	room.Mu.Lock()
	room.Players[player.Id] = player
	room.Mu.Unlock()

	welcomeMsg := internal.Message[any] {
		Type: "welcome",
		Data: map[string]any{
			"username": player.Username,
			"playerId": player.Id,
			"message": fmt.Sprintf("%s has joined the room.", player.Username),
		},
	}

	if err := player.Conn.WriteJSON(welcomeMsg); err != nil {
		return fmt.Errorf("failed to send welcome msg")
	}

	joinMsg := internal.Message[any] {
		Type: "player_joined",
		Data: map[string]any {
			"username": player.Username,
			"playerId": player.Id,
		},
	}

	broadcastToRoomExcept(room, joinMsg, player)
	return nil
}

func handlePixelDraw(player *internal.Player, rawData json.RawMessage) {
	player.Room.Mu.Lock()
	if player.Room.Current == nil || player != player.Room.Current {
		player.Room.Mu.RUnlock()
		return
	}

	var pixelMsg internal.PixelMessage 

	if err := json.Unmarshal(rawData, &pixelMsg); err != nil {
		log.Printf("Failed to parse pixel data: %v", err)
		return
	}

	log.Printf("Received pixel draw: type=%s", pixelMsg.Type)

	// Create message to broadcast to other players
	msg := internal.Message[any]{
		Type: "pixel_draw",
		Data: pixelMsg,
	}

	broadcastToRoomExcept(player.Room, msg, player)
}

