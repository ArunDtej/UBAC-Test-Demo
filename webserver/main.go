package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type RequestData struct {
	IP        string `json:"ip"`
	Email     string `json:"email"`
	UserAgent string `json:"user_agent"`
}

type AllowRequest struct {
	Email     string `json:"email"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
}

type AllowResponse struct {
	Allow  bool   `json:"allow"`
	Status string `json:"status"`
}

type LogRequest struct {
	IPAddress    string `json:"ip_address"`
	Email        string `json:"email"`
	UserAgent    string `json:"user_agent"`
	Username     string `json:"username"`
	EventType    string `json:"event_type"`
	HTTPMethod   string `json:"http_method"`
	Endpoint     string `json:"endpoint"`
	Timestamp    string `json:"timestamp"`
	ResponseCode int    `json:"response_code"`
	TrackRequest bool   `json:"track_request"`
}

var requestsSent int = 0
var requestsAllowed int = 0
var requestsBlocked int = 0

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	for {
		sent, allowed, blocked := getStats()
		stats := map[string]int{
			"requests_sent":    sent,
			"requests_allowed": allowed,
			"requests_blocked": blocked,
		}
		err := conn.WriteJSON(stats)
		if err != nil {
			log.Println(err)
			return
		}
		time.Sleep(1 * time.Second)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	requestsSent++
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var data RequestData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	allowReqPayload := AllowRequest{
		Email:     data.Email,
		IPAddress: data.IP,
		UserAgent: data.UserAgent,
	}

	payloadBytes, err := json.Marshal(allowReqPayload)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", "http://localhost:8000/api/allow", bytes.NewBuffer(payloadBytes))
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	req.Header.Set("X-API-Key", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxLCJ1c2VybmFtZSI6ImFydW50IiwidmVyc2lvbiI6MjM4ODMwfQ.PrMcrGraHFe0oDSZf2h3zZwTPNb7wT1twuSKbl2QwA0")
	// req.Header.Set("X-API-Key", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjozLCJ1c2VybmFtZSI6ImRlbW8iLCJ2ZXJzaW9uIjo5NzA2MDB9.Hi08aNpv5zRHV9v2bkW7fW6WGvIRI1MJuCE0z-nf-4A")

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var allowResp AllowResponse
	if err := json.Unmarshal(body, &allowResp); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if allowResp.Allow {

		requestsAllowed++

		statusCodes := []int{
			http.StatusOK,
			http.StatusCreated,
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusInternalServerError,
		}
		rand.Seed(time.Now().UnixNano())
		statusCode := statusCodes[rand.Intn(len(statusCodes))]

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(allowResp)

		go func(statusCode int) {
			logPayload := LogRequest{
				IPAddress:    data.IP,
				Email:        data.Email,
				UserAgent:    data.UserAgent,
				Username:     data.Email, // Using email as username as per assumption
				EventType:    "api_request",
				HTTPMethod:   r.Method,
				Endpoint:     r.URL.Path,
				Timestamp:    time.Now().UTC().Format(time.RFC3339),
				ResponseCode: statusCode,
				TrackRequest: true,
			}

			logPayloadBytes, err := json.Marshal(logPayload)
			if err != nil {
				fmt.Println("Error marshalling log payload:", err)
				return
			}

			logReq, err := http.NewRequest("POST", "http://localhost:8000/api/log", bytes.NewBuffer(logPayloadBytes))
			// logReq, err := http.NewRequest("POST", "https://api.apigate.in/api/log", bytes.NewBuffer(logPayloadBytes))
			if err != nil {
				fmt.Println("Error creating log request:", err)
				return
			}

			logReq.Header.Set("X-API-Key", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxLCJ1c2VybmFtZSI6ImFydW50IiwidmVyc2lvbiI6MjM4ODMwfQ.PrMcrGraHFe0oDSZf2h3zZwTPNb7wT1twuSKbl2QwA0")
			// logReq.Header.Set("X-API-Key", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjozLCJ1c2VybmFtZSI6ImRlbW8iLCJ2ZXJzaW9uIjo5NzA2MDB9.Hi08aNpv5zRHV9v2bkW7fW6WGvIRI1MJuCE0z-nf-4A")
			logReq.Header.Set("Content-Type", "application/json")
			logReq.Header.Set("User-Agent", "curl-test/1.0")

			logClient := &http.Client{}
			logResp, err := logClient.Do(logReq)
			if err != nil {
				fmt.Println("Error sending log request:", err)
				return
			}
			defer logResp.Body.Close()
		}(statusCode)

	} else {
		requestsBlocked++
	}
}

func getStats() (int, int, int) {
	return requestsSent, requestsAllowed, requestsBlocked
}

func main() {
	http.HandleFunc("/", handler)
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		sent, allowed, blocked := getStats()
		stats := map[string]int{
			"requests_sent":    sent,
			"requests_allowed": allowed,
			"requests_blocked": blocked,
		}
		json.NewEncoder(w).Encode(stats)
	})
	http.HandleFunc("/ws", dataHandler)
	http.Handle("/data", http.StripPrefix("/data", http.FileServer(http.Dir("../frontend"))))
	http.ListenAndServe(":8080", nil)

}
