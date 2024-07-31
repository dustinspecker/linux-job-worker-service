package job

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	cpuPeriod      = 1_000_000
	cgroupFileMode = 0o500
)

// createCGroup creates a new cgroup with the given cpu, io, and memory limits.
// No validation on the limits is done since it's expected that the caller has already validated the input.
func createCGroup(cgroupDir string, rootDeviceMajMin string, cpu float64, ioInBytes int64, memoryInBytes int64) error {
	cgroupTasksDir := filepath.Join(cgroupDir, "tasks")

	// create a directory structure like /sys/fs/cgroup/<uuid>/tasks
	if err := os.MkdirAll(cgroupTasksDir, cgroupFileMode); err != nil {
		return fmt.Errorf("error creating new control group: %w", err)
	}

	// instruct the cgroup subtree to enable cpu, io, and memory controllers
	if err := os.WriteFile(filepath.Join(cgroupDir, "cgroup.subtree_control"), []byte("+cpu +io +memory"), cgroupFileMode); err != nil {
		return fmt.Errorf("error writing cgroup.subtree_control: %w", err)
	}

	cpuQuota := int(cpu * float64(cpuPeriod))
	cpuMaxContent := fmt.Sprintf("%d %d", cpuQuota, cpuPeriod)

	if err := os.WriteFile(filepath.Join(cgroupTasksDir, "cpu.max"), []byte(cpuMaxContent), cgroupFileMode); err != nil {
		return fmt.Errorf("error writing cpu.max: %w", err)
	}

	if err := os.WriteFile(filepath.Join(cgroupTasksDir, "memory.max"), []byte(strconv.FormatInt(memoryInBytes, 10)), cgroupFileMode); err != nil {
		return fmt.Errorf("error writing memory.max: %w", err)
	}

	// TODO/Future Consideration: add support for specifying rbps, wbps, riops, and wiops for a list of devices
	formattedIOInBytes := strconv.FormatInt(ioInBytes, 10)
	ioMaxContent := fmt.Sprintf("%s rbps=%s wbps=%s riops=max wiops=max", rootDeviceMajMin, formattedIOInBytes, formattedIOInBytes)

	if err := os.WriteFile(filepath.Join(cgroupTasksDir, "io.max"), []byte(ioMaxContent), cgroupFileMode); err != nil {
		return fmt.Errorf("error writing io.max: %w", err)
	}

	return nil
}

// cleanupCGroup removes the cgroup directory and all of its contents.
func cleanupCGroup(cgroupDir string) error {
	cgroupTasksDir := filepath.Join(cgroupDir, "tasks")

	if err := os.RemoveAll(cgroupTasksDir); err != nil {
		return fmt.Errorf("error removing cgroup tasks directory: %w", err)
	}

	if err := os.RemoveAll(cgroupDir); err != nil {
		return fmt.Errorf("error removing cgroup directory: %w", err)
	}

	return nil
}
