package command

import (
	"fmt"
	"log"
)

// ExitError represents a process exit code from the CLI.
type ExitError struct {
	// Code is the process exit status.
	Code int
}

// Error returns a message for the exit code.
func (e ExitError) Error() string {
	switch e.Code {
	case 0:
		return ""
	case 1:
		return "invalid command"
	case 2:
		return "request error"
	case 3:
		return "server error"
	default:
		log.Printf("Exit code %d should be added to the ExitError description", e.Code)
		return fmt.Sprintf("command failed (code %d)", e.Code)
	}
}
