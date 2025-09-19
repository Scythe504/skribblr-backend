package internal

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

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
	Mu             sync.RWMutex `json:"-"`
}

type PlayerSnapshot struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Score          int    `json:"score"`
	IsReady        bool   `json:"is_ready"`
	HasGuessed     bool   `json:"has_guessed"`
	IsConnected    bool   `json:"is_connected"`
	CanDraw        bool   `json:"can_draw"`
	TotalGuesses   int    `json:"total_guesses"`
	CorrectGuesses int    `json:"correct_guesses"`
	TimesDrawn     int    `json:"times_drawn"`
}


func (p *Player) ResetRoundState() {
	p.HasGuessed = false
	p.CanDraw = false
	p.LastGuessTime = time.Time{}
}

func (p *Player) ToPublicPlayer() *Player {
	return &Player{
		Id:             p.Id,
		Username:       p.Username,
		Score:          p.Score,
		IsReady:        p.IsReady,
		HasGuessed:     p.HasGuessed,
		IsConnected:    p.IsConnected,
		CanDraw:        p.CanDraw,
		TotalGuesses:   p.TotalGuesses,
		CorrectGuesses: p.CorrectGuesses,
		TimesDrawn:     p.TimesDrawn,
		JoinedAt:       p.JoinedAt,
	}
}

func CreatePlayerSnapshot(p *Player) PlayerSnapshot {
	return PlayerSnapshot{
		ID:             p.Id,
		Username:       p.Username,
		Score:          p.Score,
		IsReady:        p.IsReady,
		HasGuessed:     p.HasGuessed,
		IsConnected:    p.IsConnected,
		CanDraw:        p.CanDraw,
		TotalGuesses:   p.TotalGuesses,
		CorrectGuesses: p.CorrectGuesses,
		TimesDrawn:     p.TimesDrawn,
	}
}


func (p *Player) SafeWriteJSON(v any) error {
	p.Mu.Lock()
	defer p.Mu.Unlock()
	return p.Conn.WriteJSON(v)
}