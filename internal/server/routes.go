package server

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/scythe504/skribblr-backend/internal"
	"github.com/scythe504/skribblr-backend/internal/utils"
	"github.com/scythe504/skribblr-backend/internal/websockets"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := mux.NewRouter()

	// Apply CORS middleware
	r.Use(s.corsMiddleware)

	r.HandleFunc("/", s.HelloWorldHandler)

	r.HandleFunc("/health", s.healthHandler)

	r.HandleFunc("/words", s.GetRandomWords)
	
	r.HandleFunc("/rooms-available", s.GetRoomToJoin)

	r.HandleFunc("/ws/{roomId}", websockets.HandleWebSocket)

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

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	jsonResp, err := json.Marshal(s.db.Health())

	if err != nil {
		log.Fatalf("error handling JSON marshal. Err: %v", err)
	}

	_, _ = w.Write(jsonResp)
}

func (s *Server) GetRandomWords(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now().UnixMilli()
	// Read words from the CSV file
	response := internal.Response{
		StatusCode:    http.StatusOK,
		RespStartTime: startTime,
	}
	words := utils.ReadCsvFile("./word-list.csv")

	// If no words are available, return an error response
	if len(words) == 0 {
		http.Error(w, "No words available", http.StatusInternalServerError)
		return
	}

	// Select 3 random unique words
	selectedWords := make([]internal.Word, 0, 3)
	seenIndices := make(map[int]bool)

	for len(selectedWords) < 3 && len(seenIndices) < len(words) {
		randomIndex := rand.Intn(len(words))
		if seenIndices[randomIndex] {
			continue
		}
		seenIndices[randomIndex] = true
		selectedWords = append(selectedWords, words[randomIndex])
	}

	response.Data = selectedWords
	endTime := time.Now().UnixMilli()
	response.RespEndTime = endTime
	response.NetRespTime = endTime - startTime

	jsonResp, err := json.Marshal(response)

	if err != nil {
		http.Error(w, "Error generating JSON response", http.StatusInternalServerError)
		return
	}
	// Set response headers and write the JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonResp)
}

func (s *Server) GetRoomToJoin(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now().UnixMilli()
	roomId := websockets.GetJoinableRoom()
	
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
