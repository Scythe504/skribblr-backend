package game

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/scythe504/skribblr-backend/internal"
	"github.com/scythe504/skribblr-backend/internal/utils"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// =============================================================================
// GLOBAL VARIABLES
// =============================================================================

var (
	Upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	// Room management
	Rooms   = make(map[string]*internal.Room)
	RoomsMu sync.RWMutex

	// Game configuration - TODO: Make these configurable
	MaxPlayersPerRoom = 8
	MinPlayersToStart = 2
)

// =============================================================================
// WEBSOCKET CONNECTION HANDLING
// =============================================================================

// HandleWebSocket upgrades HTTP connection to WebSocket and initializes player
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// TODO:
	// 1. Upgrade connection to WebSocket
	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade failed: ", err)
		return
	}
	// 2. Extract username from query params
	username := r.URL.Query().Get("username")
	if username == "" {
		username = "Anonymous"
	}
	width, err := strconv.Atoi(r.URL.Query().Get("w"))
	if err != nil {
		return
	}
	height, err := strconv.Atoi(r.URL.Query().Get("h"))
	if err != nil {
		return
	}
	// 3. Extract roomId from URL path
	roomIdFromUrl := strings.Split(r.URL.Path, "/")
	if len(roomIdFromUrl) < 2 {
		log.Println("No room id provided")
		conn.Close()
		return
	}
	roomId := roomIdFromUrl[2]
	// 4. Create new Player struct with generated ID
	player := &internal.Player{
		Id:           utils.GenerateID(8),
		Conn:         conn,
		Username:     username,
		CanvasWidth:  width,
		CanvasHeight: height,
		Score:        0,
	}
	// 5. Call AddPlayer to join room
	if err := AddPlayer(roomId, player); err != nil {
		log.Println("Error adding player", err)
		conn.Close()
		return
	}
	// 6. Start handleMessages goroutine
	go handleMessages(player)
	// 7. Handle connection errors gracefully
}

// handleMessages processes incoming WebSocket messages for a player
func handleMessages(player *internal.Player) {
	// TODO:
	// 1. Set up defer for cleanup (close connection, remove player)
	defer func() {
		player.Conn.Close()
		removePlayer(player)
	}()
	log.Printf("Started message handler for player: %s in room: %s", player.Username, player.Room.Id)

	// 2. Start infinite loop to read messages
	for {
		_, rawMessage, err := player.Conn.ReadMessage()
		if err != nil {
			log.Printf("Read error occured during websocket message %s, %v", player.Username, err)
			break
		}
		// 3. Parse base message structure
		var baseMsg internal.Message[json.RawMessage]
		if err := json.Unmarshal(rawMessage, &baseMsg); err != nil {
			// 4. Handle parsing errors gracefully
			log.Printf("Failed to parse base message: %v", err)
			continue
		}
		// 5. Log all message activity
		log.Printf("Received message type: %s from player: %s", baseMsg.Type, player.Username)
		// 6. Route to appropriate handlers based on message type
		switch baseMsg.Type {
		// Message types to handle:
		// - "player_ready" -> HandlePlayerReady
		case "player_ready":
			var isReady bool
			if err := json.Unmarshal(baseMsg.Data, &isReady); err != nil {
				log.Println("Error parsing data, wrong json", err)
				continue
			}
			HandlePlayerReady(player, isReady)
			// - "word_selection" -> HandleWordSelection
		case "word_selection":
			var wordSelected string
			if err := json.Unmarshal(baseMsg.Data, &wordSelected); err != nil {
				log.Println("Error parsing data, wrong json", err)
				continue
			}
			HandleWordSelection(player, wordSelected)
			// - "guess" -> HandleGuessEnhanced
		case "guess_message":
			var wordSelected string
			if err := json.Unmarshal(baseMsg.Data, &wordSelected); err != nil {
				log.Println("Error parsing data, wrong json", err)
				continue
			}
			HandleGuessEnhanced(player, wordSelected)
			// - "pixel_draw" -> HandlePixelDrawEnhance
		case "pixel_draw":
			HandlePixelDrawEnhanced(player, baseMsg.Data)
			// - "clear_canvas" -> ClearCanvas
		case "clear_canvas":
			ClearCanvas(player.Room, player)
			// - "start_game" -> StartGame (host only)
		case "start_game":
			go StartGame(player.Room)
		}
	}
}
