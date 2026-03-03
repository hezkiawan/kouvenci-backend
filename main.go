package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/rs/cors"
	"google.golang.org/genai"
)

// ==========================================
// Structs for Vault Response
// ==========================================
type VaultResponse struct {
	Data struct {
		Data map[string]string `json:"data"`
	} `json:"data"`
}

// ChatRequest represents the incoming JSON from our Frontend
type ChatRequest struct {
	Message string `json:"message"`
}

// Global variable for the GenAI Client
var client *genai.Client

// ==========================================
// 1. Zero-Trust Security: Fetch Secret at Runtime
// ==========================================
func getSecretFromVault() (string, error) {
	// DYNAMIC URL: Check if K8s provided a specific Vault Address
	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr == "" {
		vaultAddr = "http://127.0.0.1:8200" // Default for local testing
	}

	// Construct the full URL
	vaultURL := fmt.Sprintf("%s/v1/secret/data/kouventa", vaultAddr)
	log.Printf("📡 Connecting to Vault at: %s", vaultURL)

	req, err := http.NewRequest("GET", vaultURL, nil)
	if err != nil {
		return "", err
	}

	// In production, use K8s Service Account Token. Here we use root.
	req.Header.Set("X-Vault-Token", "root")

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
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
// 2. Chat Handler (Updated to use SDK)
// ==========================================
func chatHandler(w http.ResponseWriter, r *http.Request) {
	var chatReq ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Use the SDK to generate content
	// We use "gemini-1.5-flash" (Stable model)
	ctx := context.Background()
	result, err := client.Models.GenerateContent(
		ctx,
		"gemma-3-27b-it",
		genai.Text(chatReq.Message),
		nil,
	)

	if err != nil {
		// Log the ACTUAL error from Google
		log.Printf("❌ Google API Error: %v", err)
		http.Error(w, "AI Service Error", http.StatusInternalServerError)
		return
	}

	// The SDK handles parsing the response safely
	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		// Extract text from the new SDK structure
		aiText := fmt.Sprintf("%v", result.Candidates[0].Content.Parts[0].Text)

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
		log.Fatalf("❌ CRITICAL SECURITY ERROR: Could not fetch API Key: %v", err)
	}
	log.Println("✅ Identity Confirmed. API Key loaded.")

	// Step 2: Initialize Google SDK
	ctx := context.Background()
	var errClient error

	// FIX: Removed the "Backend" field. Just passing the APIKey is enough.
	client, errClient = genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: secret,
	})
	if errClient != nil {
		log.Fatalf("❌ Failed to initialize Google SDK: %v", errClient)
	}

	// Step 3: Setup Router
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", chatHandler)

	handler := cors.AllowAll().Handler(mux)

	log.Println("🚀 Server listening on port 8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}
