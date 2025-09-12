package internal
// Methods (Room Struct)
func (r *Room) GetPlayerByIndex(index int) *Player {
	if index < 0 || index >= len(r.PlayerOrder) {
		return nil
	}

	playerID := r.PlayerOrder[index]
	return r.Players[playerID]
}

func (r *Room) GetNextDrawerIndex() int {
	if len(r.PlayerOrder) == 0 {
		return -1
	}

	return (r.CurrentIndex + 1) % len(r.PlayerOrder)
}

func (r *Room) GetPlayerCount() int {
	count := 0
	for _, player := range r.Players {
		if player.IsConnected {
			count++
		}
	}
	return count
}

func (r *Room) CanStartGame() bool {
	return r.GetPlayerCount() >= MinPlayersToStart
}

func (r *Room) AreAllPlayersReady() bool {
	for _, player := range r.Players {
		if player.IsConnected && !player.IsReady {
			return false
		}
	}

	return true
}

func (r *Room) ResetPlayerGuessState() {
	for _, player := range r.Players {
		player.HasGuessed = false
	}

	r.CorrectGuessers = []PlayerGuess{}
}

func (r *Room) HasEveryoneGuessed() bool {
	for _, player := range r.Players {
		if player.IsConnected && player != r.Current && !player.HasGuessed {
			return false
		}
	}

	return true
}