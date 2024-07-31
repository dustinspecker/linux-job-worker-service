package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/dustinspecker/linux-job-worker-service/pkg/mount"
)

// job-executor satisfies the requirements of JobExecutorPath binary as described
// in the [rfd](rfd/0001-linux-job-worker-service.md).
func main() {
	// TODO: validate this process is not running in the host's pid namespace (e.g. os.Getpid() == 1)
	// to avoid accidentally killing the host's processes
	if len(os.Args) < 3 {
		usage()
		os.Exit(1)
	}

	if os.Args[1] != "run" {
		usage()
		os.Exit(1)
	}

	err := run(os.Args[2], os.Args[3:])
	if err != nil {
		log.Fatalf("error running command: %v", err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s run $COMMAND $ARGUMENTS...\n", os.Args[0])
}

func run(command string, arguments []string) (err error) {
	// create an error and signal channels so that job-executor can wait for the command to finish or be stopped
	// by receiving a SIGTERM signal
	commandDone := make(chan error, 1)
	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, syscall.SIGTERM)

	// mount a new proc filesystem so that commands like `ps -ef` only show the processes in the pid namespace
	unmountProcFileSystem, err := mount.ProcFilesystem()
	if err != nil {
		return fmt.Errorf("error mounting proc: %w", err)
	}

	defer func() {
		if unmountErr := unmountProcFileSystem(); unmountErr != nil {
			// join errors in case a previous error occurred
			err = errors.Join(err, fmt.Errorf("error unmounting proc: %w", unmountErr))
		}
	}()

	cmd := exec.Command(command, arguments...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	go func() {
		commandDone <- cmd.Run()
	}()

	// wait for command to finish or for SIGTERM signal to be received
	select {
	case err := <-commandDone:
		// TODO: consider exiting with same exit code as command executed
		if err != nil {
			return fmt.Errorf("error running command: %w", err)
		}

		// ASSUME: if command exited with children processes left behind that it's desired for
		// children processes to run until Stop() is called on the job.
		// TODO/Future consideration: wait for children processes to exit in addition to just handling
		// SIGTERM.
		// Wait for SIGTERM signal to terminate children processes
		childProcessIDs, err := getChildProcessIDs()
		if err != nil {
			return fmt.Errorf("error getting children processes: %w", err)
		}

		// if there are no children processes, then there is no need to wait for SIGTERM signal
		// since the only process running has already exited
		if len(childProcessIDs) > 0 {
			// wait to receive SIGTERM signal to terminate children processes
			<-terminate

			err := terminateChildrenProcesses()
			if err != nil {
				return fmt.Errorf("error terminating children processes: %w", err)
			}
		}
	case <-terminate:
		err := terminateChildrenProcesses()
		if err != nil {
			return fmt.Errorf("error terminating children processes: %w", err)
		}
	}

	return nil
}

func getChildProcessIDs() ([]int, error) {
	// get list of children proccesses from /proc/1/task/1/children
	// send SIGTERM to each child process and wait for each child process to exit
	// job-executor should always be running as PID 1 in the pid namespace, so
	// this should always be the correct path to get the children processes
	childProcesses, err := os.ReadFile("/proc/1/task/1/children")
	if err != nil {
		return nil, fmt.Errorf("error reading children processes: %w", err)
	}

	childProcessesContent := strings.TrimSpace(string(childProcesses))
	if childProcessesContent == "" {
		return nil, nil
	}

	childProcessIDs := strings.Split(childProcessesContent, " ")
	parsedChildProcessIDs := make([]int, len(childProcessIDs))

	for index, childProcess := range childProcessIDs {
		childPID, err := strconv.Atoi(childProcess)
		if err != nil {
			return nil, fmt.Errorf("error converting child process to int: %w", err)
		}

		parsedChildProcessIDs[index] = childPID
	}

	return parsedChildProcessIDs, nil
}

func terminateChildrenProcesses() error {
	childProcessIDs, err := getChildProcessIDs()
	if err != nil {
		return fmt.Errorf("error getting children processes: %w", err)
	}

	// TODO: use golang.org/x/sync/errgroup to wait for all child processes to exit instead of using log.Fatalf here
	// If log.Fatalf is used then the process will exit immediately and the child processes will not have a chance to exit.
	// Some processes may be able to gracefully exit if given a chance. Also, log.Fatalf prevent deferred functions
	// from running.
	var waitGroup sync.WaitGroup

	for _, childPID := range childProcessIDs {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			childProcess := os.Process{
				Pid: childPID,
			}

			if err := childProcess.Signal(syscall.SIGTERM); err != nil {
				log.Fatalf("error sending signal to child process: %v", err)
			}

			// wait for the child process to exit
			// Do not worry about sending SIGKILL if the child process does not exit
			// within a certain amount of time. SIGKILL will be sent by the kernel
			// later when job-executor is eventually killed by job.Stop().
			_, err = childProcess.Wait()
			if err != nil {
				log.Fatalf("error waiting for child process to exit: %v", err)
			}
		}()
	}

	waitGroup.Wait()

	return nil
}
