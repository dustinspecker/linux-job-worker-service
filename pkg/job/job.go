package job

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/dustinspecker/linux-job-worker-service/internal"
	"github.com/google/uuid"
)

var (
	ErrJobAlreadyStarted = errors.New("job already started")
	ErrJobNotStarted     = errors.New("job not started")

	ErrInvalidRootPhysicalDeviceMajMin = errors.New("RootPhysicalDeviceMajMin must be provided")
	ErrInvalidJobExecutorPath          = errors.New("JobExecutorPath must be provided")
	ErrInvalidCommand                  = errors.New("Command must be provided")
	ErrInvalidCPU                      = errors.New("CPU must be greater than 0")
	ErrInvalidIOInBytes                = errors.New("IOInBytes must be greater than 0")
	ErrInvalidMemoryInBytes            = errors.New("MemoryInBytes must be greater than 0")
)

// Config represents the configuration for a job.
// All fields are required except for Arguments.
// JobExecutorPath is the path to a binary fulfilling the requirements of JobExecutorPath:
//
// - must be usable such as `<JobExecutorPath> run $COMMAND $ARGUMENTS...`
//
// - must mount a new proc file system before executing $COMMAND.
type Config struct {
	// RootPhysicalDeviceMajMin is the major and minor number of the root physical device to apply IOInBytes limit to.
	RootPhysicalDeviceMajMin string
	// JobExecutorPath is the path to a binary fulfilling the requirements of JobExecutorPath.
	JobExecutorPath string

	// Command is the command to run.
	Command string
	// Arguments are the arguments to pass to the command, if any.
	Arguments []string

	// CPU is the number of CPU cores to limit the job to such as 0.5 for half a CPU core.
	CPU float64
	// IOInBytes is the number of bytes per second to limit the job to read/write on the provided RootPhysicalDeviceMajMin, such as 100_000_000 for 100 MB/s.
	IOInBytes int64
	// MemoryInBytes is the number of bytes to limit the job to use, such as 1_000_000_000 for 1 GB.
	MemoryInBytes int64
}

func validateConfig(config Config) error {
	if config.RootPhysicalDeviceMajMin == "" {
		return ErrInvalidRootPhysicalDeviceMajMin
	}

	if config.JobExecutorPath == "" {
		return ErrInvalidJobExecutorPath
	}

	if config.Command == "" {
		return ErrInvalidCommand
	}

	if config.CPU <= 0 {
		return ErrInvalidCPU
	}

	if config.IOInBytes <= 0 {
		return ErrInvalidIOInBytes
	}

	if config.MemoryInBytes <= 0 {
		return ErrInvalidMemoryInBytes
	}

	return nil
}

type State string

const (
	NotStarted State = "not started"
	Running    State = "running"
	Completed  State = "completed"
	Terminated State = "terminated"
)

// Status represents the status of a job.
// State may be:
//
// - not started
//
// - running
//
// - completed
//
// - terminated
//
// Exited is true if the job has exited by invoking exit().
// ExitCode is the exit code of the job if it has exited via exit().
// ExitReason is the reason the job has errored if it has errored during execution or cleanup.
type Status struct {
	State    State
	Exited   bool
	ExitCode int
	// If multiple errors occur then they should be joined with errors.Join
	ExitReason error
}

// Job represents a Linux Job that can run a process in a semi-isolated environment.
// The process is run in a new PID, mount, and network namespace without any ability
// to reach the internet or local network.
// The Job may be started, stopped, and queried for its status.
// The Job's combined stdout and stderr may be streamed.
// Job should not be created directly, but instead via the New function.
type Job struct {
	// cmd is the command that is running the job. It's nil until the job has started.
	cmd    *exec.Cmd
	config Config
	mutex  sync.Mutex
	// output is the combined stdout and stderr of the job
	output *internal.Output
	// terminated is true if the job has been terminated via Stop()
	terminated bool
	// exitReason is the reason the job has errored if it has errored during execution or cleanup
	exitReason error
	// processState holds information about the process once it completes
	// processState is nil until the job has completed running
	processState *os.ProcessState
}

// New creates a new Job with the provided Config.
// The Job is not started until Start is called.
func New(config Config) *Job {
	output := internal.NewOutput()

	return &Job{
		config: config,
		output: output,
	}
}

// Start starts the Job in a semi-isolated environment.
// If the provided configuration is invalid, an error is returned.
// If the Job has already been started, ErrJobAlreadyStarted is returned.
// Start will create new namespaces for PID, mount, and network.
// The process is unable to reach the internet or local network.
// Start also creates a new control group for the process limiting CPU, IO, and memory.
// Start will delete the control group when the process exits.
// Start does not wait for the process to complete.
// The user running Start() should be the root user or have the necessary permissions to create namespaces and control groups.
func (j *Job) Start() error {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	// validate config
	if err := validateConfig(j.config); err != nil {
		return fmt.Errorf("error validating config: %w", err)
	}

	// validate job isn't already running
	if j.cmd != nil {
		return ErrJobAlreadyStarted
	}

	jobExecutorCommandArguments := []string{
		"run",
		j.config.Command,
	}
	jobExecutorCommandArguments = append(jobExecutorCommandArguments, j.config.Arguments...)

	cmd := exec.Command(j.config.JobExecutorPath, jobExecutorCommandArguments...)
	// combine the stdout and stderr so that the stdout and stderr are combined in the order they are written
	cmd.Stderr = j.output
	cmd.Stdout = j.output
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// CLONE_NEWPID: creates a new PID namespace preventing the process from seeing/killing host processes
		// CLONE_NEWNET: creates a new network namespace preventing the process from accessing the internet or local network
		Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		// CLONE_NEWNS: creates a new mount namespace preventing the process from impacting host mounts
		// Also, enables mounting a new proc filesystem so that command such as `ps -ef` only see the processes in the PID namespace
		Unshareflags: syscall.CLONE_NEWNS,
		// instruct cmd.Run to use the control group file descriptor, so that JobExecutorPath does not
		// have to manually add the new PID to the control group
		UseCgroupFD: true,
	}

	// create a new control group for the process
	cgroupName := uuid.New().String()
	cgroupDir := filepath.Join("/sys/fs/cgroup/", cgroupName)

	err := createCGroup(cgroupDir, j.config.RootPhysicalDeviceMajMin, j.config.CPU, j.config.IOInBytes, j.config.MemoryInBytes)
	if err != nil {
		return fmt.Errorf("error creating cgroup: %w", err)
	}

	// open the cgroup.procs file so cmd.Run can automatically add the new PID to the control group
	cgroupTasksDir := filepath.Join(cgroupDir, "tasks")

	procsFile, err := os.OpenFile(cgroupTasksDir, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("error opening cgroup.procs: %w", err)
	}

	// provide the file descriptor to cmd.Run so that it can add the new PID to the control group
	cmd.SysProcAttr.CgroupFD = int(procsFile.Fd())

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting command: %w", err)
	}

	j.cmd = cmd

	// run the command in a Goroutine so that Start can return immediately
	go func() {
		// Use cmd.Process.Wait() instead of cmd.Wait() since cmd.Wait() is not thread safe
		// and we do not want to hold the mutex while waiting for the process to exit.
		// So instead we use cmd.Process.Wait() and store the result in j.processState to mimic what
		// cmd.Wait would do.
		// This prevents concurrency issues when a user calls Start(), the command quickly exits (updating the
		// process state), and the user invokes Status().
		processState, err := j.cmd.Process.Wait()
		j.mutex.Lock()

		j.processState = processState
		if err != nil {
			j.exitReason = errors.Join(j.exitReason, fmt.Errorf("error running command: %w", err))
		}

		if err == nil && !processState.Success() {
			j.exitReason = errors.Join(j.exitReason, &exec.ExitError{ProcessState: processState})
		}
		j.mutex.Unlock()

		j.mutex.Lock()
		defer j.mutex.Unlock()
		// close the output, so that any readers of the output know the process has exited and will no longer
		// block waiting for new output
		if err := j.output.Close(); err != nil {
			j.exitReason = errors.Join(j.exitReason, fmt.Errorf("error closing output: %w", err))
		}

		// do not close the cgroup.procs file until after the process has exited
		if err := procsFile.Close(); err != nil {
			j.exitReason = errors.Join(j.exitReason, fmt.Errorf("error closing cgroup.procs: %w", err))
		}

		// cleanup the control group when the process exits
		if err := cleanupCGroup(cgroupDir); err != nil {
			j.exitReason = errors.Join(j.exitReason, fmt.Errorf("error cleaning up cgroup: %w", err))
		}
	}()

	return nil
}

// Status returns the current Status of the Job.
// TODO: handle when an outside mechanism stops the process.
func (j *Job) Status() Status {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	var (
		exited     bool
		exitCode   = -1
		exitReason = j.exitReason
		state      = Running
	)

	// handle terminated
	if j.terminated {
		return Status{
			Exited:     exited,
			ExitCode:   exitCode,
			ExitReason: nil, // don't send exitReason since it was terminated via Stop()/signal then Wait may error with no child process, which could be confusing to a user
			State:      Terminated,
		}
	}

	// handle not started
	if j.cmd == nil {
		return Status{
			Exited:     exited,
			ExitCode:   exitCode,
			ExitReason: exitReason,
			State:      NotStarted,
		}
	}

	// cmd.ProcessState will be nil if the process is still running
	if j.processState != nil {
		exited = j.processState.Exited()
		exitCode = j.processState.ExitCode()
		state = Completed
	}

	return Status{
		State:      state,
		Exited:     exited,
		ExitCode:   exitCode,
		ExitReason: j.exitReason,
	}
}

// Stream returns an OutputReader that streams the combined stdout and stderr of the Job.
// It is okay to call Stream before Start.
// It is okay to call Stream after the Job has completed.
func (j *Job) Stream() *internal.OutputReader {
	return internal.NewOutputReader(j.output)
}

// Stop stops the Job by sending SIGTERM to the process.
// If the process does not exit after 10 seconds, SIGKILL is sent.
// Process in this context refers to the JobExecutorPath process.
// Since the process runs as pid 1 in the new PID namespace, killing it
// will kill all child processes.
// If the Job has not been started, ErrJobNotStarted is returned.
func (j *Job) Stop() error {
	// TODO/future consideration: refactor this function so
	// the mutex isn't being held for 10 seconds while the process is being terminated and waited on
	j.mutex.Lock()
	defer j.mutex.Unlock()

	if j.cmd == nil {
		return ErrJobNotStarted
	}

	if err := j.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("error sending SIGTERM: %w", err)
	}

	cmdWait := make(chan error, 1)
	timer := time.NewTimer(10 * time.Second)
	// stop timer in case process exits before 10 seconds
	// it's safe to stop timer even if stopped already
	defer timer.Stop()

	go func() {
		_, err := j.cmd.Process.Wait()
		cmdWait <- err
	}()

	select {
	case <-cmdWait:
		// command exited before timer expired, so nothing to do
	case <-timer.C:
		// send SIGKILL if process is still running after timer expires
		if err := j.cmd.Process.Signal(syscall.SIGKILL); err != nil {
			return fmt.Errorf("error sending SIGKILL: %w", err)
		}
	}

	j.terminated = true

	return nil
}
