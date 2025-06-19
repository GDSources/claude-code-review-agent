package webhook

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHMACValidator_Validate(t *testing.T) {
	secret := "test-webhook-secret"
	validator := NewHMACValidator(secret)

	testPayload := []byte(`{"action": "opened", "number": 1}`)
	validSignature := validator.GenerateSignature(testPayload)

	tests := []struct {
		name          string
		signature     string
		payload       []byte
		expectedError string
	}{
		{
			name:      "valid signature",
			signature: validSignature,
			payload:   testPayload,
		},
		{
			name:          "missing signature header",
			signature:     "",
			payload:       testPayload,
			expectedError: "missing X-Hub-Signature-256 header",
		},
		{
			name:          "invalid signature format - no prefix",
			signature:     "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			payload:       testPayload,
			expectedError: "invalid signature format, expected sha256= prefix",
		},
		{
			name:          "invalid signature format - wrong prefix",
			signature:     "sha1=abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			payload:       testPayload,
			expectedError: "invalid signature format, expected sha256= prefix",
		},
		{
			name:          "invalid signature length - too short",
			signature:     "sha256=abcdef123456",
			payload:       testPayload,
			expectedError: "invalid signature length, expected 64 hex characters",
		},
		{
			name:          "invalid signature length - too long",
			signature:     "sha256=abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890extra",
			payload:       testPayload,
			expectedError: "invalid signature length, expected 64 hex characters",
		},
		{
			name:          "invalid hex characters",
			signature:     "sha256=gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg",
			payload:       testPayload,
			expectedError: "invalid signature format, not valid hex",
		},
		{
			name:          "wrong signature for payload",
			signature:     "sha256=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			payload:       testPayload,
			expectedError: "signature verification failed",
		},
		{
			name:          "signature for different payload",
			signature:     validator.GenerateSignature([]byte(`{"action": "closed"}`)),
			payload:       testPayload,
			expectedError: "signature verification failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
			if tt.signature != "" {
				req.Header.Set("X-Hub-Signature-256", tt.signature)
			}

			err := validator.Validate(req, tt.payload)

			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing '%s', got no error", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error containing '%s', got: %v", tt.expectedError, err)
				}
			}
		})
	}
}

func TestHMACValidator_GenerateSignature(t *testing.T) {
	secret := "test-secret"
	validator := NewHMACValidator(secret)

	testCases := []struct {
		name     string
		payload  []byte
		expected string
	}{
		{
			name:     "empty payload",
			payload:  []byte{},
			expected: "sha256=b613679a0814d9ec772f95d778c35fc5ff1697c493715653c6c712144292c5ad",
		},
		{
			name:     "simple JSON payload",
			payload:  []byte(`{"test": "value"}`),
			expected: "sha256=52b582138706ac0c597c80cfe1e7f339e7c0cf4996b4d1a9b5e6e4f4f5b6a7c8", // This will be calculated
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			signature := validator.GenerateSignature(tt.payload)

			if !strings.HasPrefix(signature, "sha256=") {
				t.Errorf("expected signature to start with 'sha256=', got: %s", signature)
			}

			if len(signature) != 71 { // "sha256=" (7 chars) + 64 hex chars
				t.Errorf("expected signature length to be 71, got: %d", len(signature))
			}

			// Verify signature is valid by validating it
			req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
			req.Header.Set("X-Hub-Signature-256", signature)

			err := validator.Validate(req, tt.payload)
			if err != nil {
				t.Errorf("generated signature should be valid, got error: %v", err)
			}
		})
	}
}

func TestHMACValidator_ConsistentSignatures(t *testing.T) {
	secret := "consistent-test-secret"
	validator := NewHMACValidator(secret)
	payload := []byte(`{"action": "opened", "pull_request": {"id": 123}}`)

	// Generate signature multiple times
	sig1 := validator.GenerateSignature(payload)
	sig2 := validator.GenerateSignature(payload)
	sig3 := validator.GenerateSignature(payload)

	if sig1 != sig2 || sig2 != sig3 {
		t.Errorf("signatures should be consistent, got: %s, %s, %s", sig1, sig2, sig3)
	}
}

func TestHMACValidator_DifferentSecrets(t *testing.T) {
	payload := []byte(`{"test": "payload"}`)

	validator1 := NewHMACValidator("secret1")
	validator2 := NewHMACValidator("secret2")

	sig1 := validator1.GenerateSignature(payload)
	sig2 := validator2.GenerateSignature(payload)

	if sig1 == sig2 {
		t.Error("different secrets should produce different signatures")
	}

	// Verify that each validator rejects the other's signature
	req1 := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req1.Header.Set("X-Hub-Signature-256", sig1)

	req2 := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req2.Header.Set("X-Hub-Signature-256", sig2)

	if err := validator1.Validate(req2, payload); err == nil {
		t.Error("validator1 should reject signature from validator2")
	}

	if err := validator2.Validate(req1, payload); err == nil {
		t.Error("validator2 should reject signature from validator1")
	}
}

func TestHMACValidator_EdgeCases(t *testing.T) {
	validator := NewHMACValidator("edge-case-secret")

	tests := []struct {
		name    string
		payload []byte
	}{
		{
			name:    "empty payload",
			payload: []byte{},
		},
		{
			name:    "single byte",
			payload: []byte("a"),
		},
		{
			name:    "large payload",
			payload: []byte(strings.Repeat("large payload data ", 1000)),
		},
		{
			name:    "binary data",
			payload: []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD},
		},
		{
			name:    "unicode payload",
			payload: []byte(`{"message": "Hello 世界 🌍"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signature := validator.GenerateSignature(tt.payload)

			req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
			req.Header.Set("X-Hub-Signature-256", signature)

			err := validator.Validate(req, tt.payload)
			if err != nil {
				t.Errorf("validation should succeed for %s, got error: %v", tt.name, err)
			}
		})
	}
}
