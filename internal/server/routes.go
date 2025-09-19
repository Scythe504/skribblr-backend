package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/scythe504/skribblr-backend/internal"
	"github.com/scythe504/skribblr-backend/internal/game"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := mux.NewRouter()

	// Apply CORS middleware
	r.Use(s.corsMiddleware)

	r.HandleFunc("/", s.HelloWorldHandler)

	r.HandleFunc("/rooms-available", s.GetRoomToJoin)

	r.HandleFunc("/ws/{roomId}", game.HandleWebSocket)

	return r
}

// CORS middleware
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS Headers
		w.Header().Set("Access-Control-Allow-Origin", "*") // Wildcard allows all origins
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Credentials", "false") // Credentials not allowed with wildcard origins

		// If it's a websocket upgrade, skip further CORS checks
		if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
			next.ServeHTTP(w, r)
			return
		}

		// Handle preflight OPTIONS requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) HelloWorldHandler(w http.ResponseWriter, r *http.Request) {
	resp := make(map[string]string)
	resp["message"] = "Hello World"

	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("error handling JSON marshal. Err: %v", err)
	}

	_, _ = w.Write(jsonResp)
}

func (s *Server) GetRoomToJoin(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now().UnixMilli()
	roomId := game.GetJoinableRoom()

	var resp internal.Response

	if roomId != "" {
		// Found a joinable room - SUCCESS
		resp = internal.Response{
			StatusCode:    http.StatusOK,
			RespStartTime: startTime,
			Data:          roomId,
		}
	} else {
		// No joinable room found - NOT FOUND or could create new room
		resp = internal.Response{
			StatusCode:    http.StatusNotFound,
			RespStartTime: startTime,
			Data:          "No joinable rooms available",
		}
	}

	// Calculate response times
	endTime := time.Now().UnixMilli()
	resp.RespEndTime = endTime
	resp.NetRespTime = endTime - startTime

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)

	// Send JSON response
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
