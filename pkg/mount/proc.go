package mount

import (
	"fmt"
	"syscall"
)

// ProcFilesystem mounts a proc filesystem at /proc.
// It returns a function to unmount the proc filesystem.
// The user is expected to unmount the proc filesystem by calling the returned function.
// ProcFilesystem should not be used in a host mount namespace, otherwise
// the host's proc filesystem will be messed up and require manual intervention to fix.
func ProcFilesystem() (func() error, error) {
	// TODO: validate this process is running in a new mount namespace (e.g. / is mounted as private)
	err := syscall.Mount("proc", "/proc", "proc", 0, "")
	if err != nil {
		return nil, fmt.Errorf("error mounting proc: %w", err)
	}

	unmount := func() error {
		err := syscall.Unmount("/proc", 0)
		if err != nil {
			return fmt.Errorf("error unmounting proc: %w", err)
		}

		return nil
	}

	return unmount, nil
}
