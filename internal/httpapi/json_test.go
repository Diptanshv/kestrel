package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteErrorShape(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	const (
		status  = http.StatusBadRequest
		code    = "invalid_request"
		message = "bad input"
	)
	if err := writeError(rec, status, code, message); err != nil {
		t.Fatalf("writeError: %v", err)
	}
	if rec.Code != status {
		t.Fatalf("status = %d, want %d", rec.Code, status)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Error.Code != code || body.Error.Message != message {
		t.Fatalf("body = %+v", body)
	}
}

func TestDecodeJSONRejectsTrailingGarbage(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}{}`))
	rec := httptest.NewRecorder()

	var v struct{ A int }
	err := decodeJSON(rec, req, &v)
	if err == nil {
		t.Fatal("expected error for trailing JSON")
	}
}
