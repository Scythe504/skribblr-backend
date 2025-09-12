package internal

type Message[T any] struct {
	Type string `json:"type"`
	Data T      `json:"data"`
}

type TimerUpdateData struct {
	TimeRemaining int64     `json:"time_remaining_ms"`
	Phase         GamePhase `json:"phase"`
	IsActive      bool      `json:"is_active"`
}

type PlayerJoinedData struct {
	Player      *Player `json:"player"`
	PlayerCount int     `json:"player_count"`
	CanStart    bool    `json:"can_start"`
}

type PlayerLeftData struct {
	PlayerID    string  `json:"player_id"`
	Username    string  `json:"username"`
	PlayerCount int     `json:"player_count"`
	NewDrawer   *Player `json:"new_drawer,omitempty"` // If leaving player was drawing
}

type WordSelectionData struct {
	Choices   []string `json:"choices"`
	RoomId    string   `json:"room_id"`
	Message   string   `json:"message"`
	TimeLimit int      `json:"time_limit"`
}

type MaskedWordData struct {
	RoomID       string `json:"room_id"`
	MaskedWord   string `json:"masked_word"`
}

type FinalResults struct {
    Leaderboard   []GameResultData `json:"leaderboard"`   // sorted by score
    MVP           *GameResultData  `json:"mvp,omitempty"` // highest scorer or other criteria
    FastestGuess  *GameResultData  `json:"fastest_guess,omitempty"`
    MostAccurate  *GameResultData  `json:"most_accurate,omitempty"`
    RoundsPlayed  int              `json:"rounds_played"`
    TotalPlayers  int              `json:"total_players"`
}

