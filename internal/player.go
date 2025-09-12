package internal

import "time"

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

