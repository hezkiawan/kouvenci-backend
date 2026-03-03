package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/rs/cors"
)

// ==========================================
// Structs for Vault & Gemini
// ==========================================

// VaultResponse represents the nested JSON structure returned by HashiCorp Vault (KV v2)
type VaultResponse struct {
	Data struct {
		Data map[string]string `json:"data"`
	} `json:"data"`
}

// ChatRequest represents the incoming JSON from our Frontend
type ChatRequest struct {
	Message string `json:"message"`
}

// GeminiRequest represents the payload we send to Google's API
type GeminiRequest struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

// GeminiResponse represents the JSON we get back from Google
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// Global variable to hold the secret (fetched at runtime)
var googleAPIKey string

// ==========================================
// 1. Zero-Trust Security: Fetch Secret at Runtime
// ==========================================
func getSecretFromVault() (string, error) {
	// DYNAMIC URL: Check if K8s provided a specific Vault Address
	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr == "" {
		// Default to localhost for local testing outside Docker
		vaultAddr = "http://127.0.0.1:8200"
	}

	// Construct the full URL
	vaultURL := fmt.Sprintf("%s/v1/secret/data/kouventa", vaultAddr)
	log.Printf("📡 Connecting to Vault at: %s", vaultURL)

	req, err := http.NewRequest("GET", vaultURL, nil)
	if err != nil {
		return "", err
	}

	// In a real prod env, this token is injected via the orchestrator (like Kubernetes Service Account)
	// For this simulation, we use the root token.
	req.Header.Set("X-Vault-Token", "root")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vault returned status: %d", resp.StatusCode)
	}

	var vaultResp VaultResponse
	if err := json.NewDecoder(resp.Body).Decode(&vaultResp); err != nil {
		return "", err
	}

	apiKey, ok := vaultResp.Data.Data["GOOGLE_API_KEY"]
	if !ok {
		return "", fmt.Errorf("GOOGLE_API_KEY not found in Vault secret")
	}

	return apiKey, nil
}

// ==========================================
// 2. Chat Handler
// ==========================================
func chatHandler(w http.ResponseWriter, r *http.Request) {
	var chatReq ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Construct the payload for Gemini
	geminiPayload := GeminiRequest{
		Contents: []Content{
			{Parts: []Part{{Text: chatReq.Message}}},
		},
	}
	jsonData, _ := json.Marshal(geminiPayload)

	// Send to Google
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=" + googleAPIKey
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error calling Gemini: %v", err)
		http.Error(w, "Failed to call Gemini API", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Parse Google's response
	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		log.Printf("Error parsing Gemini response: %v", err)
		http.Error(w, "Failed to parse Gemini response", http.StatusInternalServerError)
		return
	}

	// Extract the text safely
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		aiText := geminiResp.Candidates[0].Content.Parts[0].Text

		// Return JSON to frontend
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"response": aiText})
	} else {
		http.Error(w, "No response from AI", http.StatusBadGateway)
	}
}

// ==========================================
// 3. Main Entrypoint
// ==========================================
func main() {
	log.Println("🔐 KouvenCI Backend Starting...")

	// Step 1: Securely fetch credentials
	log.Println("📡 Contacting HashiCorp Vault...")
	secret, err := getSecretFromVault()
	if err != nil {
		// FAIL FAST: If we can't get the secret, we refuse to start.
		log.Fatalf("❌ CRITICAL SECURITY ERROR: Could not fetch API Key from Vault: %v", err)
	}
	googleAPIKey = secret
	log.Println("✅ Identity Confirmed. API Key loaded into memory.")

	// Step 2: Setup Router & CORS
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", chatHandler)

	// Allow All Origins for this demo (simplifies Ingress/CORS issues)
	handler := cors.AllowAll().Handler(mux)

	log.Println("🚀 Server listening on port 8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}
