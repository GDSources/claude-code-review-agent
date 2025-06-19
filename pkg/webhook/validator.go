package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

type HMACValidator struct {
	secret string
}

func NewHMACValidator(secret string) *HMACValidator {
	return &HMACValidator{
		secret: secret,
	}
}

func (v *HMACValidator) Validate(req *http.Request, body []byte) error {
	signature := req.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		return fmt.Errorf("missing X-Hub-Signature-256 header")
	}

	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("invalid signature format, expected sha256= prefix")
	}

	expectedSignature := strings.TrimPrefix(signature, "sha256=")
	if len(expectedSignature) != 64 {
		return fmt.Errorf("invalid signature length, expected 64 hex characters")
	}

	expectedBytes, err := hex.DecodeString(expectedSignature)
	if err != nil {
		return fmt.Errorf("invalid signature format, not valid hex: %w", err)
	}

	actualSignature := v.computeSignature(body)

	if !hmac.Equal(actualSignature, expectedBytes) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

func (v *HMACValidator) computeSignature(body []byte) []byte {
	mac := hmac.New(sha256.New, []byte(v.secret))
	mac.Write(body)
	return mac.Sum(nil)
}

func (v *HMACValidator) GenerateSignature(body []byte) string {
	signature := v.computeSignature(body)
	return "sha256=" + hex.EncodeToString(signature)
}
