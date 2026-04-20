package urlutil

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenURL opens u in the OS default browser. The call is fire-and-forget;
// errors from the launched process are not returned.
func OpenURL(u string) error {
	var cmd string
	switch runtime.GOOS {
	case "linux":
		cmd = "xdg-open"
	case "darwin":
		cmd = "open"
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return exec.Command(cmd, u).Start()
}
