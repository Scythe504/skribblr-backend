package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/scythe504/skribblr-backend/internal"
	"github.com/scythe504/skribblr-backend/internal/utils"
)

// =============================================================================
// GLOBAL VARIABLES
// =============================================================================

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	// Room management
	rooms   = make(map[string]*internal.Room)
	roomsMu sync.RWMutex

	// Word database - TODO: Load from file/database
	// easyWords   = []string{"cat", "dog", "sun", "car", "tree"}
	// mediumWords = []string{"elephant", "bicycle", "pizza", "guitar", "castle"}
	// hardWords   = []string{"algorithm", "philosophy", "metamorphosis", "constellation", "archaeology"}

	// Game configuration - TODO: Make these configurable
	maxPlayersPerRoom = 8
	minPlayersToStart = 2
	// maxRounds         = 3
)

// =============================================================================
// WEBSOCKET CONNECTION HANDLING
// =============================================================================

// HandleWebSocket upgrades HTTP connection to WebSocket and initializes player
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// TODO:
	// 1. Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
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
	roomId := roomIdFromUrl[1]
	// 4. Create new Player struct with generated ID
	player := internal.Player{
		Id:           utils.GenerateID(8),
		Conn:         conn,
		Username:     username,
		CanvasWidth:  width,
		CanvasHeight: height,
		Score:        0,
	}
	// 5. Call AddPlayer to join room
	if err := AddPlayer(roomId, &player); err != nil {
		log.Println("Error adding player", err)
		conn.Close()
		return
	}
	// 6. Start handleMessages goroutine
	go handleMessages(&player)
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
		case "guess":
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
			StartGame(player.Room)
		}
	}
}

// =============================================================================
// ROOM MANAGEMENT
// =============================================================================

// GetJoinableRoom returns ID of a room that can accept new players
func GetJoinableRoom() string {
	// TODO:
	// 1. Lock rooms for reading
	roomsMu.RLock()
	defer roomsMu.RUnlock()
	// 2. Iterate through existing rooms
	for _, room := range rooms {
		room.Mu.RLock()
		// 3. Check player count < maxPlayersPerRoom
		if len(room.Players) > maxPlayersPerRoom {
			continue
		}
		// 4. Check room is in lobby phase
		if room.Phase == internal.PhaseLobby {
			// 5. Return room ID or empty string if none available
			room.Mu.RUnlock()
			return room.Id
		}
		room.Mu.RUnlock()
	}
	return ""
}

// getOrCreateRoom retrieves existing room or creates new one
func getOrCreateRoom(roomId string) *internal.Room {
	// TODO:
	// 1. Lock rooms map for writing
	roomsMu.Lock()
	defer roomsMu.Unlock()
	// 2. Check if room exists
	if room, exists := rooms[roomId]; exists {
		return room
	}

	ctx, cancel := context.WithCancel(context.Background())
	// 3. If not exists, create new room with:
	rooms[roomId] = &internal.Room{
		Id: roomId,
		//    - Initialize all maps and slices
		Players:         make(map[string]*internal.Player),
		PlayersReady:    make(map[string]bool),
		CorrectGuessers: make([]internal.PlayerGuess, 0),
		PlayerOrder:     make([]string, 0),
		WordChoices:     make([]string, 0),
		Timer:           &internal.GameTimer{IsActive: false},
		Current:         nil,

		RoundStats:  make([]internal.RoundStats, 0),
		CanvasState: make([]internal.PixelMessage, 0),
		//    - Set phase to PhaseLobby
		Phase: internal.PhaseLobby,
		//    - Create context for cleanup
		Context: ctx,
		Cancel:  cancel,
		//    - Set default values
		CurrentIndex:   0,
		Word:           "",
		RoundNumber:    1,
		MaxRounds:      3,
		HasGameStarted: false,
		Mu:             sync.RWMutex{},
	}
	// 4. Return room pointer
	return rooms[roomId]
}

// AddPlayer joins a player to a room and sends initial messages
func AddPlayer(roomId string, player *internal.Player) error {
	// TODO:
	// 1. Get or create room
	room := getOrCreateRoom(roomId)
	room.Mu.Lock()
	defer room.Mu.Unlock()
	// 2. Set player.Room reference
	player.Room = room
	// 3. Add player to room.Players map
	room.Players[player.Id] = player
	// 4. Set player initial state (IsConnected=true, IsReady=false)
	player.IsConnected = true
	player.IsReady = false
	// 5. Send welcome message to joining player
	welcomeMsg := internal.Message[any]{
		Type: "player_joined",
		Data: map[string]any{
			"message":     fmt.Sprintf("Welcome %s, %s has joined.", player.Username, player.Username),
			"player_data": player,
		},
	}
	// 6. Broadcast player_joined to other players
	broadcastToRoomExcept(room, welcomeMsg, player)
	// 7. Send current game state to new player
	if err := player.Conn.WriteJSON(room.CanvasState); err != nil {
		log.Printf("Failed to receive canvas state for player: %s:%s, Error %v \n", player.Id, player.Username, err)
		return err
	}
	// 8. If game in progress, set as spectator
	// Idk about this
	// 9. Return error if room is full
	if len(room.Players) == maxPlayersPerRoom {
		return fmt.Errorf("max players reached for this room, please join another room")
	}
	return nil
}

// removePlayer handles player disconnection and cleanup
func removePlayer(player *internal.Player) {
	// TODO:
	// 1. Get player's room
	room := player.Room
	if room == nil {
		return
	}
	room.Mu.Lock()
	// 2. Remove from room.Players map
	delete(room.Players, player.Id)
	delete(room.PlayersReady, player.Id)
	// 3. Remove from room.PlayerOrder slice
	room.PlayerOrder = slices.DeleteFunc(room.PlayerOrder, func(s string) bool {
		return s == player.Id
	})
	// 4. If player was current drawer, handle gracefully:
	wasCurrentDrawer := (room.Current == player)
	playerCount := len(room.Players)
	room.Mu.Unlock()

	if wasCurrentDrawer && room.Phase == internal.PhaseDrawing {
		// - Cancel current round
		CancelPhaseTimer(room)
		// - Advance to next player
		if playerCount >= minPlayersToStart {
			NextRound(room)
		} else {
			ResetRoomToLobby(room)
		}
	} else if playerCount < minPlayersToStart && room.HasGameStarted {
		ResetRoomToLobby(room)
	}
	// 5. If too few players, pause/end game
	if playerCount == 0 {
		CleanupRoom(room)
		return
	}

	// 6. If room empty, delete from rooms map
	if len(room.Players) == 0 {
		roomsMu.Lock()
		delete(rooms, room.Id)
		roomsMu.Unlock()
	}

	// 7. Broadcast player_left message
	leaveMessage := internal.Message[any]{
		Type: "player_left",
		Data: map[string]any{
			"message":           fmt.Sprintf("%s has left the game", player.Username),
			"player_id":         player.Id,
			"username":          player.Username,
			"players_remaining": playerCount,
		},
	}
	// 8. Update game state for remaining players
	broadcastToRoom(room, leaveMessage)
	BroadcastGameState(room)
}

// CleanupRoom handles complete room shutdown
func CleanupRoom(room *internal.Room) {
	// TODO:
	// 1. Cancel room context (stops all timers)
	room.Mu.Lock()
	if room.Cancel != nil {
		room.Cancel()
	}
	// 2. Close all player connections
	for _, player := range room.Players {
		if err := player.Conn.Close(); err != nil {
			log.Printf("Error occured while closing the player connection for %s, Err:%v\n", player.Username, err)
			continue
		}
	}
	room.Mu.Unlock()
	// 3. Remove room from global rooms map
	roomsMu.Lock()
	delete(rooms, room.Id)
	roomsMu.Unlock()
	// 4. Clear all room data structures
	room.Players = nil
	room.PlayerOrder = nil
	// Kind of rudimentary since GC will cleanup once no reference to these structs/slices remains
}

// =============================================================================
// GAME FLOW - LOBBY & INITIALIZATION
// =============================================================================

// HandlePlayerReady toggles player ready state in lobby
func HandlePlayerReady(player *internal.Player, ready bool) {
	// TODO:
	room := player.Room
	room.Mu.Lock()
	defer room.Mu.Unlock()
	// 1. Verify room is in lobby phase
	if room.Phase != internal.PhaseLobby {
		log.Println("Room is not in lobby phase")
		return
	}
	// 2. Set player.IsReady = ready
	player.IsReady = ready
	// 3. Update room.PlayersReady map
	room.PlayersReady[player.Id] = ready
	// 4. Broadcast lobby state update to all players
	lobbyUpdate := internal.Message[any]{
		Type: "lobby_update",
		Data: map[string]any{
			"player_id":     player.Id,
			"username":      player.Username,
			"is_ready":      ready,
			"ready_count":   len(room.PlayersReady),
			"total_players": len(room.Players),
		},
	}
	broadcastToRoom(room, lobbyUpdate)
	// 5. Check if all connected players are ready
	allReady := room.AreAllPlayersReady()
	enoughPlayers := len(room.PlayersReady) >= minPlayersToStart
	// 6. If ready && enough players, call StartGame()
	if allReady && enoughPlayers {
		room.Mu.Unlock() // Unlocking before calling a function to prevent deadlock
		StartGame(room)
		room.Mu.Lock() // Re-locking for defer
	}
}

// StartGame initializes a new game when conditions are met
func StartGame(room *internal.Room) error {
	// TODO:
	room.Mu.Lock()
	// 1. Verify enough players (>= minPlayersToStart)
	if len(room.PlayersReady) < minPlayersToStart {
		room.Mu.Unlock()
		return fmt.Errorf("not enough players to start game: %d/%d", len(room.PlayersReady), minPlayersToStart)
	}
	// 2. Verify all players are ready
	if !room.AreAllPlayersReady() {
		room.Mu.Unlock()
		return fmt.Errorf("not all players are ready")
	}
	// 3. Initialize game state:
	//    - Set HasGameStarted = true
	room.HasGameStarted = true
	//    - Set RoundNumber = 1
	room.RoundNumber = 1
	//    - Create PlayerOrder from connected players
	room.PlayerOrder = make([]string, 0, len(room.PlayerOrder))
	for playerId, isReady := range room.PlayersReady {
		player := room.Players[playerId]
		if player != nil && player.IsConnected && isReady {
			room.PlayerOrder = append(room.PlayerOrder, playerId)
		}
	}
	//    - Set CurrentIndex = 0
	room.CurrentIndex = 0
	//    - Reset all player states
	room.ResetPlayerGuessState()
	//    - Clear scores
	room.RoundStats = make([]internal.RoundStats, 0)

	gameStartedMsg := internal.Message[any]{
		Type: "game_started",
		Data: map[string]any{
			"message": "Game has started!",
			"room_id": room.Id,
			"players_count": len(room.PlayerOrder),
			"players": room.Players,
		},
	}
	room.Mu.Unlock()
	// 4. Call StartWaitingPhase()
	StartWaitingPhase(room)
	// 5. Broadcast game_started message
	broadcastToRoom(room, gameStartedMsg)
	return nil
}

// ResetRoomToLobby returns room to waiting-for-players state
func ResetRoomToLobby(room *internal.Room) {
	// TODO:
	room.Mu.Lock()
	// 1. Cancel all active timers
	CancelPhaseTimer(room)
	// 2. Set Phase = PhaseLobby
	room.Phase = internal.PhaseLobby
	// 3. Set HasGameStarted = false
	room.HasGameStarted = false
	// 4. Reset all game state variables
	room.CorrectGuessers = make([]internal.PlayerGuess, 0)
	room.Word = ""
	room.RoundNumber = 1
	room.WordChoices = make([]string, 0, 3)
	room.Current = nil
	room.CurrentIndex = 0
	room.PlayerOrder = make([]string, 0)
	// 5. Set all players IsReady = false
	room.PlayersReady = make(map[string]bool)
	for playerId := range room.Players {
		room.Players[playerId].IsReady = false
		room.Players[playerId].ResetRoundState()
	}
	// 6. Clear scores, round stats, canvas state
	room.CanvasState = make([]internal.PixelMessage, 0)
	room.RoundStats = make([]internal.RoundStats, 0)
	for playerID := range room.Players {
		room.Players[playerID].Score = 0
	}
	// 7. Broadcast lobby_reset message
	lobbyResetMessage := internal.Message[any]{
		Type: "lobby_reset",
		Data: map[string]any{
			"message":          fmt.Sprintf("Lobby %s has been reset for new game", room.Id),
			"room_id":          room.Id,
			"timestamp":        time.Now().UnixMilli(),
			"players":          room.Players,
			"phase":            room.Phase,
			"current_drawer":   room.Current,
			"round_number":     room.RoundNumber,
			"max_rounds":       room.MaxRounds,
			"correct_guessers": room.CorrectGuessers,
		},
	}
	room.Mu.Unlock()
	broadcastToRoom(room, lobbyResetMessage)
}

// =============================================================================
// GAME FLOW - ROUND MANAGEMENT
// =============================================================================

// StartWaitingPhase shows next drawer countdown (10 seconds)
func StartWaitingPhase(room *internal.Room) {
	// TODO:
	room.Mu.Lock()
	// 1. Set room.Phase = PhaseWaiting
	room.Phase = internal.PhaseWaiting
	// 2. Get next drawer
	if room.CurrentIndex >= len(room.PlayerOrder) {
		room.CurrentIndex = 0 // Reset if invalid
	}
	// 3. Set room.Current = next drawer
	if len(room.PlayerOrder) > 0 {
		playerId := room.PlayerOrder[room.CurrentIndex]
		room.Current = room.Players[playerId]
	} else {
		room.Mu.Unlock()
		log.Printf("No players in PlayerOrder for room %s", room.Id)
		return
	}
	// 4. Reset all players' round state:
	for _, player := range room.Players {
		// - HasGuessed = false
		player.HasGuessed = false
		// - CanDraw = false
		player.CanDraw = false
	}
	// 5. Clear CorrectGuessers slice
	room.CorrectGuessers = make([]internal.PlayerGuess, 0)
	// 6. Clear canvas state
	room.CanvasState = make([]internal.PixelMessage, 0)

	// Prepare waiting message
	waitingPhaseMessage := internal.Message[any]{
		Type: "waiting_phase",
		Data: map[string]any{
			"message": fmt.Sprintf("%s will draw next, selecting word...", room.Current.Username),
			"room_id": room.Id,
			"current_drawer": map[string]string{
				"id":       room.Current.Id,
				"username": room.Current.Username,
			},
			"phase":          "waiting",
			"time_remaining": 10, // 10 seconds waiting
			"round_number": room.RoundNumber,
		},
	}
	room.Mu.Unlock()
	// 8. Broadcast waiting_phase message with next drawer info
	broadcastToRoom(room, waitingPhaseMessage)
	// 7. Start 15-second timer with callback to StartWordSelection
	StartPhaseTimer(room, 15*time.Second, func() {
		StartWordSelection(room)
	})
}

// StartWordSelection presents 3 word choices to current drawer
func StartWordSelection(room *internal.Room) {
	// TODO:
	room.Mu.Lock()
	// Check if room state is still valid
	if room.Current == nil {
		room.Mu.Unlock()
		log.Printf("No current drawer in room %s", room.Id)
		return
	}
	// 1. Generate 3 words using GenerateWordChoices()
	words := utils.GenerateWordChoices()
	// 2. Store in room.WordChoices (temporary)
	room.WordChoices = words
	currentDrawer := room.Current
	room.Mu.Unlock()

	// 3. Send word_selection message ONLY to current drawer
	wordSelectionMessage := internal.Message[internal.WordSelectionData]{
		Type: "word_selection",
		Data: internal.WordSelectionData{
			Message:   "Please select a word to draw",
			RoomId:    room.Id,
			Choices:   words,
			TimeLimit: 15, // seconds
		},
	}

	if err := currentDrawer.Conn.WriteJSON(wordSelectionMessage); err != nil {
		log.Printf("Failed to send word selection to drawer %s: %v", currentDrawer.Username, err)
		// Auto-select first word if can't send
		HandleWordSelection(currentDrawer, words[0])
		return
	}

	// 6. Broadcast "waiting for word selection" to other players
	waitingMessage := internal.Message[any]{
		Type: "waiting_for_word",
		Data: map[string]any{
			"message":        fmt.Sprintf("Waiting for %s to select a word...", currentDrawer.Username),
			"current_drawer": currentDrawer.Username,
			"time_remaining": 15,
		},
	}
	broadcastToRoomExcept(room, waitingMessage, currentDrawer)

	// 4. Start 15-second selection timer
	StartPhaseTimer(room, 15*time.Second, func() {
		// Auto-select first word if no selection made
		room.Mu.RLock()
		// 5. Set auto-selection callback if no response
		if len(room.WordChoices) > 0 && room.Word == "" {
			autoWord := room.WordChoices[0]
			room.Mu.RUnlock()
			log.Printf("Auto-selecting word '%s' for drawer %s", autoWord, currentDrawer.Username)
			HandleWordSelection(currentDrawer, autoWord)
		} else {
			room.Mu.RUnlock()
		}
	})
}

// HandleWordSelection processes drawer's word choice
func HandleWordSelection(player *internal.Player, selectedWord string) {
	// TODO:
	room := player.Room
	room.Mu.Lock()
	// 1. Verify player is current drawer
	if player != room.Current {
		log.Println("Not the current drawer", player.Id, room.Id)
		room.Mu.Unlock()
		return
	}
	// 2. Verify selectedWord exists in room.WordChoices
	if !slices.Contains(room.WordChoices, selectedWord) {
		log.Printf("Player: %s, has chosen an invalid word, roomId: %s", player.Id, room.Id)
		room.Mu.Unlock()
		return
	}
	// 3. Set room.Word = selectedWord
	room.Word = selectedWord
	// 4. Clear room.WordChoices (security - don't leak choices)
	room.WordChoices = make([]string, 0)
	// 5. Cancel word selection timer
	room.Mu.Unlock()
	CancelPhaseTimer(room)
	// 6. Call StartDrawingPhase()
	StartDrawingPhase(room)
}

// StartDrawingPhase begins main drawing/guessing gameplay (75 seconds)
func StartDrawingPhase(room *internal.Room) {
	room.Mu.Lock()
	// 1. Set room.Phase = PhaseDrawing
	room.Phase = internal.PhaseDrawing
	// 2. Set current drawer's CanDraw = true
	room.Current.CanDraw = true

	// 3. Clear any previous correct guessers for this round
	room.CorrectGuessers = make([]internal.PlayerGuess, 0)

	// Reset all players' HasGuessed status
	for _, player := range room.Players {
		if player != nil {
			player.HasGuessed = false
		}
	}
	room.Mu.Unlock()

	// 4. Start the phase timer - this handles all timer logic including:
	//    - Setting up timer state, context, goroutines
	//    - Broadcasting timer updates every second
	//    - Calling onExpire callback when time runs out
	StartPhaseTimer(room, internal.DrawingPhaseDuration, func() {
		// Check if everyone guessed before time expired
		room.Mu.RLock()
		allGuessed := room.HasEveryoneGuessed()
		room.Mu.RUnlock()

		if allGuessed {
			StartRevealingPhase(room)
		} else {
			NextRound(room)
		}
	})

	// 5. Broadcast drawing_phase message
	room.Mu.RLock()
	// Masked word for guessers (e.g., "_ _ _ _ _")
	maskedWord := internal.MaskedWordData{
		RoomID:     room.Id,
		MaskedWord: utils.GetMaskedWord(room.Word),
	}
	maskedWordMessage := internal.Message[any]{
		Type: "drawing_phase",
		Data: maskedWord,
	}
	room.Mu.RUnlock()

	// Send masked word to all players except drawer
	broadcastToRoomExcept(room, maskedWordMessage, room.Current)

	// Send full word and drawer info to the drawer
	room.Mu.RLock()
	drawerData := internal.Message[any]{
		Type: "drawing_phase",
		Data: map[string]any{
			"room_id":        room.Id,
			"current_word":   room.Word,
			"current_drawer": room.Current,
			"phase":          internal.PhaseDrawing,
			"time_remaining": int64(internal.DrawingPhaseDuration.Seconds()),
		},
	}
	room.Mu.RUnlock()

	if err := room.Current.Conn.WriteJSON(drawerData); err != nil {
		log.Printf("Error sending drawer data to %s, roomId: %s: %v", room.Current.Id, room.Id, err)
	}
}

// StartRevealingPhase shows word and round results (8 seconds)
func StartRevealingPhase(room *internal.Room) {
	// TODO:
	room.Mu.Lock()
	// 1. Set room.Phase = PhaseRevealing
	room.Phase = internal.PhaseRevealing
	// 2. Cancel drawing timer
	if room.Timer.Cancel != nil {
		room.Timer.Cancel()
	}
	// 3. Set all players CanDraw = false
	for _, player := range room.Players {
		player.CanDraw = false
	}
	// 4. Calculate final scores for round
	// 5. Award drawer points based on correct guesses
	// Idk how exactly maybe a new function (CalculateDrawerPoints)
	// 6. Create RoundStats entry
	roundStats := internal.RoundStats{
		RoundNumber:    room.RoundNumber,
		DrawerId:       room.Current.Id,
		Word:           room.Word,
		CorrectGuesses: room.CorrectGuessers,
		TotalGuesses:   len(room.CorrectGuessers),
		StartTime:      room.Timer.StartTime,
		EndTime:        time.Now(),
	}
	room.RoundStats = append(room.RoundStats, roundStats)

	nIndex := room.GetNextDrawerIndex()
	nextPlayer := room.GetPlayerByIndex(nIndex)

	var finalScores []*internal.Player = make([]*internal.Player, 0)
	for _, player := range room.Players {
		finalScores = append(finalScores, player)
	}

	// Check if game should end
	var isGameEnded bool
	// isGameEnded = shouldGameEnd(room)

	//  Create final scores list (sorted by score)
	// finalScores := getSortedPlayerScores(room)

	// 7. Broadcast round_end message:
	roundEndData := internal.RoundEndData{
		Word:            room.Word,
		DrawerID:        room.Current.Id,
		CorrectGuessers: room.CorrectGuessers,
		DrawerUsername:  room.Current.Username,
		NextDrawer:      nextPlayer,
		FinalScores:     finalScores,
		IsGameEnded:     false, // Correct me: I need to check if the drawer == len(room.PlayerOrders)-1  and current round == maxRounds
	}
	roundEndMessage := internal.Message[any]{
		Type: "round_end",
		Data: roundEndData,
	}

	room.Mu.Unlock()
	broadcastToRoom(room, roundEndMessage)
	// 8. Start 8-second reveal timer
	onRevealComplete := func() {
		if isGameEnded {
			EndGame(room)
		} else {
			NextRound(room)
		}
	}
	// 9. Set callback: NextRound() or EndGame()
	StartPhaseTimer(room, 8*time.Second, onRevealComplete)
}

// NextRound advances to next player or ends game
func NextRound(room *internal.Room) {
	room.Mu.Lock()
	defer room.Mu.Unlock()

	// 1. Rebuild player order (handles join/leave safely)
	utils.UpdatePlayerOrder(room)

	if len(room.PlayerOrder) == 0 {
		// No players left, just end the game
		go EndGame(room)
		return
	}

	// 2. Move to the next index (with wraparound)
	room.CurrentIndex = (room.CurrentIndex + 1) % len(room.PlayerOrder)

	// 3. If we wrapped back to the first player, increment round
	if room.CurrentIndex == 0 {
		room.RoundNumber++
		if room.RoundNumber > room.MaxRounds {
			go EndGame(room)
			return
		}
	}

	// 4. Assign new drawer
	nextPlayerID := room.PlayerOrder[room.CurrentIndex]
	room.Current = room.Players[nextPlayerID]

	// ✅ 5. Validate game state after mutations
	if !utils.ValidateGameState(room) {
		log.Printf("[NextRound] Invalid game state detected in room %s", room.Id)
	}

	// Unlock before starting the next phase
	go StartWaitingPhase(room)
}

// EndGame finishes game and shows final results
func EndGame(room *internal.Room) {
	// TODO:
	room.Mu.Lock()
	// 1. Set room.Phase = PhaseEnded
	room.Phase = internal.PhaseEnded
	// 2. Cancel all active timers
	CancelPhaseTimer(room)
	// 3. Calculate final rankings/statistics
	room.Mu.Unlock()
	resultData := CalculateFinalResults(room)
	// 4. Broadcast game_ended message:
	//    - Final leaderboard
	//    - Game statistics
	//    - MVP/awards
	resultMessage := internal.Message[any]{
		Type: "game_ended",
		Data: resultData,
	}
	broadcastToRoom(room, resultMessage)
	// 5. Start 30-second timer to return to lobby
	// 6. Set callback: ResetRoomToLobby()
	StartPhaseTimer(room, 30*time.Second, func() {
		ResetRoomToLobby(room)
	})
}

// =============================================================================
// TIMER MANAGEMENT
// =============================================================================

// StartPhaseTimer creates and manages a phase timer with regular updates
func StartPhaseTimer(room *internal.Room, duration time.Duration, onExpire func()) {
	// TODO:
	room.Mu.Lock()
	// 1. Cancel any existing timer using CancelPhaseTimer()
	CancelPhaseTimer(room)
	// 2. Create new context with cancellation
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	// 3. Create GameTimer struct:
	room.Timer = &internal.GameTimer{
		//    - StartTime = now
		StartTime: time.Now(),
		//    - Duration = provided duration
		Duration: duration,
		//    - IsActive = true
		IsActive: true,
		//    - Context and Cancel function
		Context: ctx,
		Cancel:  cancel,
	}
	room.Mu.Unlock()
	// 4. Start goroutine:
	go func() {
		//    - Use time.NewTicker(1 * time.Second) for updates
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				//    - Call BroadcastTimerUpdate() every second
				BroadcastTimerUpdate(room)
				//    - When time expires, call onExpire()
			case <-ctx.Done():
				//    - Handle context cancellation for cleanup
				onExpire()
				return
			}
		}
	}()
}

// BroadcastTimerUpdate sends current timer state to all players
func BroadcastTimerUpdate(room *internal.Room) {
	// TODO:
	room.Mu.Lock()
	if room.Timer == nil || room.Timer.IsActive {
		room.Mu.Unlock()
		return
	}
	remaining := room.Timer.Duration - time.Since(room.Timer.StartTime)
	if remaining < 0 {
		remaining = 0
	}
	// 2. Create TimerUpdateData struct
	timerUpdateData := internal.TimerUpdateData{
		TimeRemaining: remaining.Milliseconds(),
		Phase:         room.Phase,
		IsActive:      room.Timer.IsActive,
	}

	room.Timer.TimeRemaining = remaining

	timerUpdateMessage := internal.Message[any]{
		Type: "timer_update",
		Data: timerUpdateData,
	}
	// 3. Broadcast timer_update message to all players
	room.Mu.Unlock()
	broadcastToRoom(room, timerUpdateMessage)
}

// CancelPhaseTimer stops current phase timer
func CancelPhaseTimer(room *internal.Room) {
	// TODO:
	room.Mu.Lock()
	// 1. Check if room.Timer exists and is active
	if room.Timer != nil && !room.Timer.IsActive {
		room.Mu.Unlock()
		return
	}
	// 2. Call timer.Cancel() to stop goroutine
	room.Timer.Cancel()
	// 3. Set timer.IsActive = false
	room.Timer.TimeRemaining = 0
	room.Timer.IsActive = false
	// 4. Optional: broadcast timer stopped message
	timerUpdateData := internal.TimerUpdateData{
		TimeRemaining: room.Timer.TimeRemaining.Milliseconds(),
		Phase:         room.Phase,
		IsActive:      room.Timer.IsActive,
	}

	timerUpdateMessage := internal.Message[any]{
		Type: "timer_update",
		Data: timerUpdateData,
	}
	room.Mu.Unlock()
	broadcastToRoom(room, timerUpdateMessage)
}

// =============================================================================
// GUESS HANDLING
// =============================================================================

// HandleGuessEnhanced processes player guesses with enhanced scoring
func HandleGuessEnhanced(player *internal.Player, guess string) {
	// TODO:
	room := player.Room
	room.Mu.Lock()
	// 1. Verify game is in drawing phase
	if room.Phase != internal.PhaseDrawing {
		room.Mu.Unlock()
		return
	}
	// 2. Verify player is not the current drawer
	if player == room.Current {
		room.Mu.Unlock()
		return
	}
	// 3. Verify player hasn't already guessed correctly
	if player.HasGuessed {
		room.Mu.Unlock()
		return
	}
	// 4. Clean guess (trim, lowercase)
	cleanedGuess := strings.ToLower(strings.Trim(guess, " "))
	// 5. Check if guess matches room.Word
	if room.Word != cleanedGuess {
		// - Increment player.TotalGuesses (stats tracking)
		player.TotalGuesses++
		playerGuess := internal.PlayerGuess{
			PlayerID:  player.Id,
			Username:  player.Username,
			GuessTime: int(time.Now().UnixMilli()),
			IsCorrect: false,
		}

		room.Mu.Unlock()
		// - Broadcast guess to chat (visible if player hasn't guessed yet)
		guessMessage := internal.Message[any]{
			Type: "guess_message",
			Data: playerGuess,
		}
		// - If player already guessed correctly, skip broadcasting
		broadcastToRoom(room, guessMessage)
		return
	}

	// 6. If correct:
	//    - Calculate points using CalculateGuessPoints()
	timeTaken := time.Since(room.Timer.StartTime)
	position := len(room.CorrectGuessers) + 1

	// Determine WordDifficulty
	var diff internal.WordDifficulty
	switch {
	case len(room.Word) >= 3 && len(room.Word) <= 5:
		diff = internal.DifficultyEasy
	case len(room.Word) > 5 && len(room.Word) <= 8:
		diff = internal.DifficultyMedium
	case len(room.Word) > 8:
		diff = internal.DifficultyHard
	}

	points := CalculateGuessPoints(timeTaken, position, diff)

	//    - Add player to CorrectGuessers
	playerGuess := internal.PlayerGuess{
		PlayerID:  player.Id,
		Username:  player.Username,
		GuessTime: int(timeTaken),
		IsCorrect: true,
	}
	room.CorrectGuessers = append(room.CorrectGuessers, playerGuess)

	//    - Award points to both guesser and drawer
	player.Score += points
	player.TotalGuesses++
	player.CorrectGuesses++

	room.Current.Score += 50
	//    - Set player.HasGuessed = true
	player.HasGuessed = true
	//    - Broadcast guess_result message
	resultData := internal.GameResultData{
		PlayerID:    player.Id,
		Username:    player.Username,
		IsCorrect:   true,
		Score:       points,
		Position:    position,
		TimeToGuess: timeTaken.Milliseconds(),
	}

	room.Mu.Unlock()

	resultMessage := internal.Message[any]{
		Type: "guess_result",
		Data: resultData,
	}
	broadcastToRoom(room, resultMessage)
	//    - Check if everyone guessed (end round early)
	room.Mu.Lock()
	if room.HasEveryoneGuessed() {
		room.Mu.Unlock()
		CancelPhaseTimer(room)
		NextRound(room)
		return
	}
	room.Mu.Unlock()
}

// CalculateGuessPoints determines points based on speed, position, and difficulty
func CalculateGuessPoints(timeTaken time.Duration, position int, wordDifficulty internal.WordDifficulty) int {
	// TODO:
	// 1. Set base points by difficulty:
	t := timeTaken.Seconds()
	p := position
	var basePoints int
	finalPoints := 0
	switch wordDifficulty {
	//    - Easy: 100 points
	case internal.DifficultyEasy:
		basePoints = 100
		//    - Medium: 150 points
	case internal.DifficultyMedium:
		basePoints = 150
		//    - Hard: 200 points
	case internal.DifficultyHard:
		basePoints = 200
	}

	// 2. Apply speed bonus (faster = more points):
	var speedMultiplier float32
	switch {
	case t >= 0 && t < 10:
		//    - < 10s: 150% multiplier
		speedMultiplier = 1.5
	case t >= 10 && t < 30:
		//    - < 30s: 125% multiplier
		speedMultiplier = 1.25
	case t >= 30 && t < 60:
		//    - < 60s: 100% multiplier
		speedMultiplier = 1
	case t > 60:
		//    - > 60s: 75% multiplier
		speedMultiplier = 0.75
	}

	// 3. Apply position penalty:
	var posMultiplier float32
	switch p {
	case 1:
		//    - 1st: 100% of calculated points
		posMultiplier = 1
	case 2:
		//    - 2nd: 80% of calculated points
		posMultiplier = 0.8
	case 3:
		//    - 3rd: 60% of calculated points
		posMultiplier = 0.6
	default:
		//    - 4th+: 40% of calculated points
		posMultiplier = 0.4
	}
	// 4. Return final calculated points
	return int(float32(basePoints) + (float32(finalPoints) * speedMultiplier * posMultiplier))
}

// =============================================================================
// DRAWING SYSTEM
// =============================================================================

// HandlePixelDrawEnhanced processes drawing with permission verification
func HandlePixelDrawEnhanced(player *internal.Player, rawData json.RawMessage) {
	// TODO: 0. Get room reference
	room := player.Room

	// TODO: 1. Lock room mutex for concurrency safety
	room.Mu.Lock()
	defer room.Mu.Unlock() // Unlock at the end

	// TODO: 2. Verify game is in drawing phase
	if room.Phase != internal.PhaseDrawing {
		// - If not, return early
		return
	}

	// TODO: 3. Verify player is the current drawer
	if room.Current != player {
		// - If not, return early
		return
	}

	// TODO: 4. Verify player.CanDraw is true
	if !player.CanDraw {
		// - If not, return early
		return
	}

	// TODO: 5. Parse rawData into PixelMessage struct
	var pixelMessage internal.PixelMessage
	if err := json.Unmarshal(rawData, &pixelMessage); err != nil {
		// - Handle errors gracefully
		log.Println("Malformed pixelMessage json obj", err)
		// - If malformed JSON, return early
		return
	}

	// TODO: 6. Validate pixel data
	switch pixelMessage.Type {
	case internal.PixelPlace, internal.ErasePixel:
		if pixelMessage.X == nil || pixelMessage.Y == nil {
			return
		}

		// - Check bounds against server canonical canvas
		if *pixelMessage.X < 0 || *pixelMessage.X >= internal.CanvasWidth ||
			*pixelMessage.Y < 0 || *pixelMessage.Y >= internal.CanvasHeight {
			// - Pixel out of bounds, discard
			return
		}
	case internal.BatchPlace, internal.BatchErase:
		validPixels := []internal.GridPosition{}
		for _, p := range pixelMessage.Pixels {
			if p.GridX < 0 || p.GridX >= internal.CanvasWidth ||
				p.GridY < 0 || p.GridY >= internal.CanvasHeight {
				continue
			}
			validPixels = append(validPixels, p)
		}
		pixelMessage.Pixels = validPixels
		if len(pixelMessage.Pixels) == 0 {
			// Nothing to draw/erase
			return
		}
	}

	// TODO: 7. Normalize coordinates
	// - Use server canonical grid (GridWidth x GridHeight)
	// - If client sent scaled coordinates, convert to grid positions
	// - Maintain aspect ratio
	switch pixelMessage.Type {
	case internal.PixelPlace, internal.ErasePixel:
		gridX, gridY := internal.NormalizeCoordinates(*pixelMessage.X, *pixelMessage.Y, player.CanvasWidth, player.CanvasHeight)
		pixelMessage.X = &gridX
		pixelMessage.Y = &gridY
	case internal.BatchPlace, internal.BatchErase:
		for i, p := range pixelMessage.Pixels {
			gridX, gridY := internal.NormalizeCoordinates(p.GridX, p.GridY, player.CanvasWidth, player.CanvasHeight)
			pixelMessage.Pixels[i].GridX = gridX
			pixelMessage.Pixels[i].GridY = gridY
		}
	}

	if pixelMessage.Timestamp == 0 {
		pixelMessage.Timestamp = time.Now().UnixMilli()
	}

	// TODO: 8. Apply changes to room.CanvasState
	switch pixelMessage.Type {
	// - Single pixel: append/update canvas
	case internal.PixelPlace:
		room.CanvasState = append(room.CanvasState, pixelMessage)
	case internal.BatchPlace:
		room.CanvasState = append(room.CanvasState, pixelMessage)
		// - Batch: loop through each pixel and append/update
	case internal.ErasePixel:
		newCanvas := []internal.PixelMessage{}
		for _, existing := range room.CanvasState {
			if existing.Type == internal.PixelPlace &&
				existing.X != nil && existing.Y != nil &&
				*existing.X == *pixelMessage.X && *existing.Y == *pixelMessage.Y {
				continue
			}
			newCanvas = append(newCanvas, existing)
		}
		// - Erase operations: remove pixels from canvas
		room.CanvasState = newCanvas
	case internal.BatchErase:
		eraseMap := map[string]struct{}{}
		for _, p := range pixelMessage.Pixels {
			key := fmt.Sprintf("%d_%d", p.GridX, p.GridY)
			eraseMap[key] = struct{}{}
		}

		newCanvas := []internal.PixelMessage{}
		for _, exisiting := range room.CanvasState {
			if exisiting.Type == internal.PixelPlace && exisiting.X != nil && exisiting.Y != nil {
				key := fmt.Sprintf("%d_%d", *exisiting.X, *exisiting.Y)
				if _, ok := eraseMap[key]; ok {
					continue
				}
			}
			newCanvas = append(newCanvas, exisiting)
		}
		room.CanvasState = newCanvas
	}

	// TODO: 9. Broadcast pixel draw message to other players
	// - Keep type: PixelPlace, BatchPlace, ErasePixel, BatchErase
	// - Send normalized grid positions, not client pixel positions
	pixelDrawMessage := internal.Message[any]{
		Type: string(pixelMessage.Type),
		Data: pixelMessage,
	}

	// TODO: 10. Optional: throttle or rate-limit broadcasts
	// - Avoid flooding network for large batch operations

	// TODO: 12. Unlock room.Mu before broadcasting
	// - Broadcasting can be outside lock to avoid blocking other actions
	go broadcastToRoomExcept(room, pixelDrawMessage, room.Current)
}

// ClearCanvas resets the drawing canvas
func ClearCanvas(room *internal.Room, clearedBy *internal.Player) {
	// TODO:
	room.Mu.Lock()
	// 1. Verify clearedBy is current drawer (or allow anyone?)
	if room.Current != clearedBy {
		room.Mu.Unlock()
		return
	}
	// 2. Clear room.CanvasState slice
	room.CanvasState = make([]internal.PixelMessage, 0)
	// 3. Broadcast canvas_cleared message to all players
	clearedCanvasMessage := internal.Message[map[string]any]{
		Type: "canvas_cleared",
		Data: map[string]any{
			"room_id":      room.Id,
			"player_id":    clearedBy.Id,
			"canvas_state": room.CanvasState,
			"timestamp":    time.Now().UnixMilli(),
		},
	}
	room.Mu.Unlock()
	broadcastToRoomExcept(room, clearedCanvasMessage, clearedBy)
	// 4. Log canvas clear action
	utils.LogGameEvent(room, clearedCanvasMessage.Type, clearedCanvasMessage.Data)
}

// UpdateDrawingPermissions sets who can draw based on game state
func UpdateDrawingPermissions(room *internal.Room) {
	// TODO:
	room.Mu.Lock()
	// 1. Set all players CanDraw = false by default
	for id := range room.Players {
		room.Players[id].CanDraw = false
	}
	// 2. If in drawing phase, set current drawer CanDraw = true
	if room.Phase == internal.PhaseDrawing {
		room.Current.CanDraw = true
	}
	// 3. Broadcast drawing_permissions_updated message
	drawingPermissionMessage := internal.Message[map[string]any]{
		Type: "drawing_permission_updated",
		Data: map[string]any{
			"room_id":   room.Id,
			"player_id": room.Current.Id,
			"message":   fmt.Sprintf("%s is now going to draw.", room.Current.Username),
		},
	}
	room.Mu.Unlock()
	broadcastToRoom(room, drawingPermissionMessage)
}

// =============================================================================
// BROADCASTING & MESSAGING
// =============================================================================

// broadcastToRoom sends message to all players in room
func broadcastToRoom[T any](room *internal.Room, msg internal.Message[T]) {
	// TODO:
	room.Mu.RLock()
	// 1. Get snapshot of players with read lock
	playersSnapshot := make([]*internal.Player, 0, len(room.Players))
	for _, p := range room.Players {
		// 3. Skip nil players or nil connections

		if p != nil && p.Conn != nil {
			playersSnapshot = append(playersSnapshot, p)
		}
	}
	room.Mu.RUnlock() // Release RLock early to avoid blocking

	// 2. Iterate through players
	// Iterate over the snapshot and send messages
	for _, player := range playersSnapshot {
		if err := player.Conn.WriteJSON(msg); err != nil {
			// Handle websocket close or other errors
			if websocket.IsCloseError(err) {
				go removePlayer(player) // Remove asynchronously to avoid deadlock
			}
			// Log errors, but continue with other players
			utils.LogGameEvent(room, "broadcast_error", map[string]any{
				"player_id": player.Id,
				"error":     err.Error(),
				"msg_type":  msg.Type,
			})
		} else {
			// Successful send logging
			utils.LogGameEvent(room, msg.Type, msg.Data)
		}
	}
	// 6. Use proper locking to avoid races
}

// broadcastToRoomExcept sends message to all players except one
func broadcastToRoomExcept[T any](room *internal.Room, msg internal.Message[T], excludePlayer *internal.Player) {
	// TODO:
	// 1. Similar to broadcastToRoom
	room.Mu.RLock()
	playersSnapshot := make([]*internal.Player, 0, len(room.Players))
	for _, p := range room.Players {
		if p != nil && p.Conn != nil && p != excludePlayer {
			playersSnapshot = append(playersSnapshot, p)
		}
	}
	room.Mu.RUnlock() // Release RLock early to avoid blocking

	// TODO:
	// 2. Skip the excludePlayer in iteration
	for _, player := range playersSnapshot {
		// TODO:
		// 3. Handle same error cases and logging
		if err := player.Conn.WriteJSON(msg); err != nil {
			if websocket.IsCloseError(err) {
				go removePlayer(player) // Remove asynchronously to avoid deadlock
			}
			utils.LogGameEvent(room, "broadcast_error", map[string]any{
				"player_id": player.Id,
				"error":     err.Error(),
				"msg_type":  msg.Type,
			})
		} else {
			utils.LogGameEvent(room, msg.Type, msg.Data)
		}
	}
}

// BroadcastGameState sends complete game state to all players
func BroadcastGameState(room *internal.Room) {
	// Validate first to avoid sending broken state
	if !utils.ValidateGameState(room) {
		log.Printf("[BroadcastGameState] Invalid game state in room %s, skipping broadcast", room.Id)
		return
	}

	room.Mu.RLock()
	// 1. Create GameStateData struct with:
	baseState := internal.GameStateData{}
	//    - Current phase
	baseState.Phase = room.Phase
	//    - Round information
	baseState.RoundNumber = room.RoundNumber
	//    - Player list (use ToPublicPlayer() to avoid sensitive data)
	for _, p := range room.Players {
		baseState.Players = append(baseState.Players, p.ToPublicPlayer())
	}
	//    - Current drawer info
	if room.Current != nil {
		baseState.CurrentDrawer = room.Current.ToPublicPlayer()
	}
	//    - Timer information
	if room.Timer != nil {
		baseState.TimeRemaining = int64(room.Timer.TimeRemaining)
	}
	//    - Masked word (if in drawing phase)
	if baseState.Phase == internal.PhaseDrawing {
		baseState.Word = utils.GetMaskedWord(room.Word)
	}
	room.Mu.RUnlock() // unlock early, snapshot is safe

	// Copy for guessers (masked word)
	guesserState := baseState
	gameStateUpdateGuessers := internal.Message[internal.GameStateData]{
		Type: "game_state_update",
		Data: guesserState,
	}

	// Copy for drawer (full word)
	drawerState := baseState
	drawerState.Word = room.Word
	gameStateUpdateDrawer := internal.Message[internal.GameStateData]{
		Type: "game_state_update",
		Data: drawerState,
	}

	// 2. Send different data based on player role:
	if room.Current != nil {
		if err := room.Current.Conn.WriteJSON(gameStateUpdateDrawer); err != nil {
			utils.LogGameEvent(room, gameStateUpdateDrawer.Type, map[string]any{
				"game_state_data": drawerState,
				"err":             err.Error(),
			})
			if websocket.IsCloseError(err) {
				go removePlayer(room.Current)
			}
		}
	}

	// 3. Broadcast game_state_update message
	broadcastToRoomExcept(room, gameStateUpdateGuessers, room.Current)
}
