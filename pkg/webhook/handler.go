package webhook

import (
	"fmt"
	"io"
	"net/http"
)

type Validator interface {
	Validate(req *http.Request, body []byte) error
}

type EventProcessor interface {
	Process(eventType string, payload []byte) error
}

type Handler struct {
	validator      Validator
	eventProcessor EventProcessor
	maxBodySize    int64
}

func NewHandler(validator Validator, eventProcessor EventProcessor) *Handler {
	return &Handler{
		validator:      validator,
		eventProcessor: eventProcessor,
		maxBodySize:    1024 * 1024, // 1MB default
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "Missing X-GitHub-Event header", http.StatusBadRequest)
		return
	}

	if r.ContentLength > h.maxBodySize {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxBodySize))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if err := h.validator.Validate(r, body); err != nil {
		http.Error(w, fmt.Sprintf("Validation failed: %v", err), http.StatusUnauthorized)
		return
	}

	if err := h.eventProcessor.Process(eventType, body); err != nil {
		http.Error(w, fmt.Sprintf("Failed to process event: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
