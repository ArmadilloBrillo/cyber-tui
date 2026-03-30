package rtdb

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// ParseRTDBToken decodes the RTDB JWT payload (without signature verification)
// and returns the Firebase project ID from the "aud" claim.
func ParseRTDBToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("rtdb: malformed JWT: expected 3 parts, got %d", len(parts))
	}

	// Add padding if needed — JWT base64 uses raw (unpadded) encoding.
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	b, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		// Try URL encoding as a fallback.
		b, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return "", fmt.Errorf("rtdb: cannot base64-decode JWT payload: %w", err)
		}
	}

	var claims map[string]any
	if err := json.Unmarshal(b, &claims); err != nil {
		return "", fmt.Errorf("rtdb: cannot parse JWT payload JSON: %w", err)
	}

	aud, ok := claims["aud"]
	if !ok {
		return "", fmt.Errorf("rtdb: JWT payload missing 'aud' claim")
	}

	switch v := aud.(type) {
	case string:
		if v == "" {
			return "", fmt.Errorf("rtdb: 'aud' claim is empty")
		}
		return v, nil
	default:
		return "", fmt.Errorf("rtdb: 'aud' claim has unexpected type %T", aud)
	}
}

// BaseURL derives the Firebase RTDB base URL from a project ID.
// Uses the modern "-default-rtdb" URL format (projects created 2021+).
func BaseURL(projectID string) string {
	return "https://" + projectID + "-default-rtdb.firebaseio.com"
}
