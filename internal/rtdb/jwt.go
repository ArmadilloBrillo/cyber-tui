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
		// The project ID is interpolated into the RTDB hostname, so reject any
		// value outside the Firebase project-ID charset to prevent a crafted
		// token from redirecting RTDB traffic to another host.
		if !validProjectID(v) {
			return "", fmt.Errorf("rtdb: 'aud' claim %q is not a valid project id", v)
		}
		return v, nil
	default:
		return "", fmt.Errorf("rtdb: 'aud' claim has unexpected type %T", aud)
	}
}

// validProjectID reports whether s contains only the characters allowed in a
// Firebase project ID (ASCII letters, digits, and hyphen).
func validProjectID(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return s != ""
}

// BaseURL derives the Firebase RTDB base URL from a project ID.
// Uses the modern "-default-rtdb" URL format (projects created 2021+).
func BaseURL(projectID string) string {
	return "https://" + projectID + "-default-rtdb.firebaseio.com"
}
