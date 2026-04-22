package urlutil

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenURL opens u in the OS default browser. The call is fire-and-forget;
// errors from the launched process are not returned.
func OpenURL(u string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		// "start" requires an empty title arg when the URL contains & characters.
		return exec.Command("cmd", "/c", "start", "", u).Start()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}
