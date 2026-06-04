package urlutil

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"unicode"
)

// HTTPURL reports whether u is an absolute http or https URL with a host and no
// control characters. Only such URLs are safe to hand to the OS opener; other
// schemes (file:, javascript:, mailto:, OS-registered custom handlers) must
// never be launched from untrusted post content.
func HTTPURL(u string) bool {
	if strings.ContainsFunc(u, unicode.IsControl) {
		return false
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

// OpenURL opens u in the OS default browser. It refuses anything that is not a
// plain http or https URL. The call is fire-and-forget; errors from the launched
// process are not returned.
func OpenURL(u string) error {
	if !HTTPURL(u) {
		return fmt.Errorf("urlutil: refusing to open non-http(s) url")
	}
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		// Avoid `cmd /c start`: cmd.exe re-parses the URL and treats &, |, ^, <, >
		// as command separators. FileProtocolHandler receives the URL as a single
		// argument under standard Windows quoting, so query strings stay intact.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return fmt.Errorf("urlutil: unsupported OS: %s", runtime.GOOS)
	}
}
