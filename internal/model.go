package internal

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	WaitingPhaseDuration   = 15 * time.Second
	DrawingPhaseDuration   = 120 * time.Second
	RevealingPhaseDuration = 8 * time.Second
	MaxPlayersPerRoom      = 8
	MinPlayersToStart      = 2
	MaxRounds              = 3
)

type GamePhase string

const (
	PhaseWaiting   GamePhase = "waiting"
	PhaseDrawing   GamePhase = "drawing"
	PhaseRevealing GamePhase = "revealing"
	PhaseEnded     GamePhase = "ending"
	PhaseLobby     GamePhase = "lobby"
)

type WordDifficulty string

const (
	DifficultyEasy   WordDifficulty = "easy"
	DifficultyMedium WordDifficulty = "medium"
	DifficultyHard   WordDifficulty = "hard"
)

type Word struct {
	Word      string         `json:"word"`
	Count     int            `json:"count"`
	Difficult WordDifficulty `json:"difficulty"`
	Points    int            `json:"points"`
}

type GameTimer struct {
	StartTime     time.Time     `json:"start_time"`
	Duration      time.Duration `json:"duration"`
	TimeRemaining time.Duration `json:"time_remaining"`
	IsActive      bool          `json:"is_active"`
	Context       context.Context
	Cancel        context.CancelFunc
}

type PlayerGuess struct {
	PlayerID  string `json:"player_id"`
	Username  string `json:"username"`
	GuessTime int    `json:"guess_time"`
	IsCorrect bool   `json:"is_correct"`
}

type RoundStats struct {
	RoundNumber    int           `json:"round_number"`
	DrawerId       string        `json:"drawer_id"`
	Word           string        `json:"word"`
	CorrectGuesses []PlayerGuess `json:"correct_guesses"`
	TotalGuesses   int           `json:"total_guesses"`
	StartTime      time.Time     `json:"start_time"`
	EndTime        time.Time     `json:"end_time"`
}

type Response struct {
	StatusCode    int   `json:"status_code"`
	RespStartTime int64 `json:"resp_time_start_ms"`
	RespEndTime   int64 `json:"resp_time_end_ms"`
	NetRespTime   int64 `json:"net_resp_time_ms"`
	Data          any   `json:"data"`
}

type Room struct {
	Id      string
	Players map[string]*Player

	// Game State
	Phase        GamePhase `json:"phase"`
	Current      *Player   `json:"current_drawer"`
	CurrentIndex int       `json:"current_index"`
	Word         string    `json:"word"`
	WordChoices  []string  `json:"word_choices,omitempty"` //Only available for current drawer

	// Round Management
	RoundNumber int          `json:"round_number"`
	MaxRounds   int          `json:"max_rounds"`
	RoundStats  []RoundStats `json:"round_stats"`

	// Timer
	Timer *GameTimer `json:"timer"`

	// Player Order and Management
	PlayerOrder  []string        `json:"player_order"`
	PlayersReady map[string]bool `json:"players_ready"`

	// Guessing State
	CorrectGuessers []PlayerGuess `json:"correct_guessers"`
	HasGameStarted  bool          `json:"has_game_started"`

	// Drawing Canvas State
	CanvasState []PixelMessage `json:"canvas_state,omitempty"`

	// Concurrency control
	Mu sync.RWMutex `json:"-"`

	// Context for cleanup
	Context context.Context    `json:"-"`
	Cancel  context.CancelFunc `json:"-"`
}

type Player struct {
	Id       string          `json:"id"`
	Conn     *websocket.Conn `json:"-"`
	Room     *Room           `json:"-"` // Avoid circular reference in JSON
	Username string          `json:"username"`
	Score    int             `json:"score"`

	// Game state
	CanvasHeight  int       `json:"canvas_height"`
	CanvasWidth   int       `json:"canvas_width"`
	IsReady       bool      `json:"is_ready"`
	HasGuessed    bool      `json:"has_guessed"`
	LastGuessTime time.Time `json:"last_guess_time"`
	IsConnected   bool      `json:"is_connected"`
	JoinedAt      time.Time `json:"joined_at"`

	// DrawingPermissions
	CanDraw bool `json:"can_draw"`

	// Statistics
	TotalGuesses   int `json:"total_guesses"`
	CorrectGuesses int `json:"correct_guesses"`
	TimesDrawn     int `json:"times_drawn"`
}

type GameStateData struct {
	Phase           GamePhase     `json:"phase"`
	RoundNumber     int           `json:"round_number"`
	MaxRounds       int           `json:"max_rounds"`
	CurrentDrawer   *Player       `json:"current_drawer"`
	TimeRemaining   int64         `json:"time_remaining"`
	Players         []*Player     `json:"players"`
	CorrectGuessers []PlayerGuess `json:"correct_guessers"`
	Word            string        `json:"word,omitempty"`
}

type GameResultData struct {
	PlayerID    string `json:"player_id"`
	Username    string `json:"username"`
	IsCorrect   bool   `json:"is_correct"`
	Score       int    `json:"score"`
	Position    int    `json:"position"`
	TimeToGuess int64  `json:"time_to_guess_ms"`
}

type RoundEndData struct {
	Word            string        `json:"word"`
	DrawerID        string        `json:"drawer_id"`
	DrawerUsername  string        `json:"drawer_username"`
	CorrectGuessers []PlayerGuess `json:"correct_guessers"`
	NextDrawer      *Player       `json:"next_drawer"`
	FinalScores     []*Player     `json:"final_scores"`
	RoundNumber     int           `json:"round_number"`
	IsGameEnded     bool          `json:"is_game_ended"`
}
