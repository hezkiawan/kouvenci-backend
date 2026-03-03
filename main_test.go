package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	// 1. Create a request to pass to our handler.
	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Create a ResponseRecorder (which mimics http.ResponseWriter)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(healthHandler)

	// 3. Call the handler directly
	handler.ServeHTTP(rr, req)

	// 4. Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// 5. Check the response body is what we expect.
	expected := `{"status":"alive"}`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}
