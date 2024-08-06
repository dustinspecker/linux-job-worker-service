package integration_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dustinspecker/linux-job-worker-service/pkg/job"
)

func TestRunningJob(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("error getting current working directory: %v", err)
	}

	config := job.Config{
		RootPhysicalDeviceMajMin: getMajMinForRootDevice(t),
		JobExecutorPath:          filepath.Join(cwd, "..", "..", "bin", "job-executor"),
		Command:                  "echo",
		Arguments:                []string{"hello", "world"},
		CPU:                      0.5,           // half a CPU core
		IOInBytes:                100_000_000,   // 100 MB/s
		MemoryInBytes:            1_000_000_000, // 1 GB
	}

	testJob := job.New(config)

	// start the job
	err = testJob.Start()
	if err != nil {
		t.Fatalf("error starting job: %v", err)
	}

	// wait for the job to finish by waiting for io.ReadAll to complete
	output, err := io.ReadAll(testJob.Stream())
	if err != nil {
		t.Fatalf("error reading output: %v", err)
	}

	status := testJob.Status()

	if status.State != "completed" {
		t.Errorf("expected job state to be 'completed', got '%s'", status.State)
	}

	if status.ExitCode != 0 {
		t.Errorf("expected job exit code to be 0, got %d", status.ExitCode)
	}

	if !status.Exited {
		t.Error("expected job to have exited")
	}

	if status.ExitReason != nil {
		t.Errorf("expected job exit reason to be nil, got %v", status.ExitReason)
	}

	if string(output) != "hello world\n" {
		t.Fatalf("expected output to be 'hello world\n', got '%s'", output)
	}
}

func TestJobPreventsNetworkRequests(t *testing.T) {
	// Prove that the job-executor binary is not able to make network requests by showing that ping
	// to localhost fails since the loopback device is not turned on.
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("error getting current working directory: %v", err)
	}

	config := job.Config{
		RootPhysicalDeviceMajMin: getMajMinForRootDevice(t),
		JobExecutorPath:          filepath.Join(cwd, "..", "..", "bin", "job-executor"),
		Command:                  "ping",
		Arguments:                []string{"-c", "1", "127.0.0.1"},
		CPU:                      0.5,           // half a CPU core
		IOInBytes:                100_000_000,   // 100 MB/s
		MemoryInBytes:            1_000_000_000, // 1 GB
	}

	testJob := job.New(config)

	// start the job
	err = testJob.Start()
	if err != nil {
		t.Fatalf("error starting job: %v", err)
	}

	// wait for the job to finish by waiting for io.ReadAll to complete
	output, err := io.ReadAll(testJob.Stream())
	if err != nil {
		t.Fatalf("error reading output: %v", err)
	}

	status := testJob.Status()

	if status.State != "completed" {
		t.Errorf("expected job state to be 'completed', got '%s'", status.State)
	}

	if status.ExitCode != 1 {
		t.Errorf("expected job exit code to be 1, got %d", status.ExitCode)
	}

	if !status.Exited {
		t.Error("expected job to have exited")
	}

	if status.ExitReason == nil {
		t.Errorf("expected job exit reason to be set when command errors, but got nil")
	}

	expectedPingOutput := "Network is unreachable"
	if !strings.Contains(string(output), expectedPingOutput) {
		t.Fatalf("expected output to contain %q, got %q", expectedPingOutput, output)
	}
}

func TestStoppingLongLivedJob(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("error getting current working directory: %v", err)
	}

	config := job.Config{
		RootPhysicalDeviceMajMin: getMajMinForRootDevice(t),
		JobExecutorPath:          filepath.Join(cwd, "..", "..", "bin", "job-executor"),
		Command:                  "/bin/bash",
		Arguments:                []string{"-c", "sleep infinity"},
		CPU:                      0.5,           // half a CPU core
		IOInBytes:                100_000_000,   // 100 MB/s
		MemoryInBytes:            1_000_000_000, // 1 GB
	}

	testJob := job.New(config)

	// start the job
	err = testJob.Start()
	if err != nil {
		t.Fatalf("error starting job: %v", err)
	}

	// validate status while job is running
	status := testJob.Status()

	if status.State != "running" {
		t.Errorf("expected job state to be 'running', got '%s'", status.State)
	}

	if status.ExitCode != -1 {
		t.Errorf("expected job exit code to be -1, got %d", status.ExitCode)
	}

	if status.Exited {
		t.Error("expected job to not have exited")
	}

	if status.ExitReason != nil {
		t.Errorf("expected job exit reason to not be set when command is still running")
	}

	// TODO: find a better workaround
	// This is a hack to ensure the job-executor's signal handler has time to be set up.
	// Otherwise, job-executor misses the SIGTERM signal.
	time.Sleep(1 * time.Second)

	// stop job
	err = testJob.Stop()
	if err != nil {
		t.Errorf("error stopping job: %v", err)
	}

	// wait for job to completely stop
	_, err = io.ReadAll(testJob.Stream())
	if err != nil {
		t.Errorf("error reading output: %v", err)
	}

	status = testJob.Status()
	if status.State != "terminated" {
		t.Errorf("expected job state to be 'terminated', got '%s'", status.State)
	}

	if status.ExitCode != -1 {
		t.Errorf("expected job exit code to be -1, got %d", status.ExitCode)
	}

	if status.Exited {
		t.Error("expected job to not have exited since it was terminated via a signal")
	}

	if status.ExitReason != nil {
		t.Errorf("expected job exit reason to be nil with no errors while waiting for command to finish, but got %v", status.ExitReason)
	}
}

func TestSpawnedChildrenProcesses(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("error getting current working directory: %v", err)
	}

	config := job.Config{
		RootPhysicalDeviceMajMin: getMajMinForRootDevice(t),
		JobExecutorPath:          filepath.Join(cwd, "..", "..", "bin", "job-executor"),
		Command:                  "/bin/bash",
		Arguments:                []string{"-c", "sleep infinity &"},
		CPU:                      0.5,           // half a CPU core
		IOInBytes:                100_000_000,   // 100 MB/s
		MemoryInBytes:            1_000_000_000, // 1 GB
	}

	testJob := job.New(config)

	// start the job
	err = testJob.Start()
	if err != nil {
		t.Fatalf("error starting job: %v", err)
	}

	// TODO: find a better workaround
	// This is a hack to ensure the job-executor's signal handler has time to be set up.
	// Otherwise, job-executor misses the SIGTERM signal.
	time.Sleep(1 * time.Second)

	// stop job
	err = testJob.Stop()
	if err != nil {
		t.Errorf("error stopping job: %v", err)
	}

	// wait for job to completely stop
	_, err = io.ReadAll(testJob.Stream())
	if err != nil {
		t.Errorf("error reading output: %v", err)
	}

	status := testJob.Status()
	if status.State != "terminated" {
		t.Errorf("expected job state to be 'terminated', got '%s'", status.State)
	}

	if status.ExitCode != -1 {
		t.Errorf("expected job exit code to be -1, got %d", status.ExitCode)
	}

	if status.Exited {
		t.Error("expected job to not have exited since it was terminated via a signal")
	}

	if status.ExitReason != nil {
		t.Errorf("expected job exit reason to be nil with no errors while waiting for command to finish, but got %v", status.ExitReason)
	}
}

func TestIOLimits(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("error getting current working directory: %v", err)
	}

	config := job.Config{
		RootPhysicalDeviceMajMin: getMajMinForRootDevice(t),
		JobExecutorPath:          filepath.Join(cwd, "..", "..", "bin", "job-executor"),
		Command:                  "dd",
		Arguments:                []string{"if=/dev/zero", "of=/tmp/file1", "bs=64M", "count=1", "oflag=direct"},
		CPU:                      0.5,           // half a CPU core
		IOInBytes:                10_000_000,    // 10 MB/s
		MemoryInBytes:            1_000_000_000, // 1 GB
	}

	testJob := job.New(config)

	// start the job
	err = testJob.Start()
	if err != nil {
		t.Fatalf("error starting job: %v", err)
	}

	// get job output
	output, err := io.ReadAll(testJob.Stream())
	if err != nil {
		t.Errorf("error reading output: %v", err)
	}

	// TODO: this test hasn't appeared flaky yet, but there's probably a better way to validate the output/IO limit
	expectedIOLimitOutput := "10.0 MB/s"
	if !strings.Contains(string(output), expectedIOLimitOutput) {
		t.Errorf("expected output to contain %q, got %q", expectedIOLimitOutput, output)
	}
}

// TODO: test cpu limits

func getMajMinForRootDevice(t *testing.T) string {
	t.Helper()

	rootDeviceMajMin := os.Getenv("ROOT_DEVICE_MAJ_MIN")
	if rootDeviceMajMin == "" {
		t.Fatal("ROOT_DEVICE_MAJ_MIN environment variable must be set")
	}

	return rootDeviceMajMin
}
