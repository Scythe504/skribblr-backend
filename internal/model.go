package internal

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Word struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

type Response struct {
	StatusCode    int         `json:"status_code"`
	RespStartTime int64       `json:"resp_time_start_ms"`
	RespEndTime   int64       `json:"resp_time_end_ms"`
	NetRespTime   int64       `json:"net_resp_time_ms"`
	Data          interface{} `json:"data"`
}

type Room struct {
	Id      string
	Players map[string]*Player
	Current *Player
	Word    string
	Mu      sync.RWMutex
}

type Player struct {
	Id       string
	Conn     *websocket.Conn
	Room     *Room
	Username string
	Score    int
}

// type SentMessage struct {
// 	User Player
// }

type Message struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type DrawData struct {
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Color      string `json:"color"`
	LineWidth  int    `json:"lineWidth"`
	IsDragging bool   `json:"isDragging"`
}
