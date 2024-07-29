---
authors: Dustin Specker
state: draft
---

# RFD 1 - Linux Job Worker Service

## What

Design and API for starting arbitrary Linux processes (jobs), streaming job output, querying job status, and stopping
jobs.

## Why

A user may want to run a variety of Linux processes on a machine, such as:

- short-lived commands like `df -h`
- long-lived commands like `sleep 1000`

A job's process should be isolated for security purposes and to prevent disrupting
other running workloads.

A user may want to query the status of a job to determine if it's still running or
stream the process's output as it runs.

## User Stories

A user wants to run a short-lived command such as `df -h`, verify the command ran successfully, and view the entirety of the command's output.

A user wants to run a long-lived command such as `sleep 1000`, verify the command is running, and stream the command's output from when it started to current output.

A user wants to identify why a job failed to run and view the error message by checking the status of the job.

A user starts a long-lived job and decides they no longer need it. The user wants to stop the job. The user later wants to view
the job's output afterward.

## Details

The Linux job worker service is made up of the following components:

- a library to run isolated processes with resource limits
- a gRPC API to manage jobs and invoke processes using the above library
- a CLI to communicate with the API via gRPC using mTLS for authentication and authorization

### Assumptions

- only support a single Linux 64-bit machine
- a new enough kernel to support cgroups v2 and namespaces
- cgroups v2 is set up on the machine
- the machine has enough memory to hold all running jobs' stdout and stderr in memory

### Out of Scope

- high availability of the server
- observability of the server
- role based access control
- restarting jobs
- other Linux namespaces such as UTS or user
- usage of pivot root or creating a new root filesystem
- other cgroup configuration such as pid limits

### CLI/UX

A user will interact with the job worker service via a CLI.

#### Starting a job

An example of running a command:

```bash
job-worker-client start \
  -ca-cert-path /path/to/ca.crt \
  -client-cert-path /path/to/client.crt \
  -client-key-path /path/to/client.key \
  -cpu 0.5 \
  -memBytes 1000000000 \
  -ioBytesPerSecond 1000000 \
  /bin/bash -c "for ((i=0; i<100; i++)); do echo \"hello \$i\"; sleep 1; done"
```


which prints something like:

```
starting job <uuid>
```

Note: The `memBytes` flag is the maximum amount of memory to be used by the job. The example above sets the memory limit to 1GB.
The `-cpu` flag is an appromixate number of CPU cores to limit the job to. The `-ioBytesPerSecond` limits read and write I/O
operations to that number of bytes per second.

The user must take note of the returned UUID to query the job's status, stream the job's output, or stop the job.

#### Querying a job's status

```bash
job-worker-client status \
  -ca-cert-path /path/to/ca.crt \
  -client-cert-path /path/to/client.crt \
  -client-key-path /path/to/client.key \
  <uuid>
```

If the job is running then it prints something like:

```
status: running
exit code: -1
exit reason:
```

If the job has completed, then it prints something like:

```
status: complete
exit code: 0
exit reason:
```

If the job was killed, then it prints something like:

```
status: killed
exit code: -1
exit reason:
```

If the job failed to run, then it prints something like:

```
status: killed
exit code: -1
exit reason: exec: "not-a-command": executable file not found in $PATH
```

#### Streaming a job's output

The `stream` command prints the entirety of the job's stdout and stderr combined to stdout from when the job started until
current.

```bash
job-worker-client stream \
  -ca-cert-path /path/to/ca.crt \
  -client-cert-path /path/to/client.crt \
  -client-key-path /path/to/client.key \
  <uuid>
```

which prints the combined stdout and stderr of the process such as:

```
hello 0
hello 1
hello 2
hello 3
```

The command will continue to print the output until the job completes or is killed.

In the case that the job has completed, the command will print the output and then exit.

#### Stopping a job

```bash
job-worker-client stop \
  -ca-cert-path /path/to/ca.crt \
  -client-cert-path /path/to/client.crt \
  -client-key-path /path/to/client.key \
  <uuid>
```

which prints something like:

```
job <uuid> stopped
```

### API

The API is a gRPC service written in Golang. The API acts as a wrapper around the library to start, query, stream, and stop jobs.

### How the server works

Note: gRPC handlers each run in their own goroutine. Out of the box, this enables the server to support concurrent requests.
The handlers must be safe for concurrent use, such as using a map with a mutex to store jobs.

#### Starting a new job

When the server receives a request to start a new job, it will:
1. create a new `Job` using the library
1. generate an ID being a UUID v4
   - Note: this UUID is completely separate from any UUID generated in the library for cgroup paths
1. store the `Job` in a map with the job's UUID as the key, along with the user ID that created the job
1. start the `Job`
1. return the job's UUID to the client

#### Querying a job's status

When the server receives a request to query a job's status, it will:
1. Look up the job in the map by the job's UUID
1. Return the job's status to the client

#### Streaming a job's output

When the server receives a request to stream a job's output, it will:
1. Look up the job in the map by the job's UUID
1. Invoke the `Stream` function on the `Job` using the library
1. Send the output to the client as it is received

#### Stopping a job

When the server receives a request to stop a job, it will:
1. Look up the job in the map by the job's UUID
1. Invoke the `Stop` function on the `Job` using the library

The server does nothing else to clean up jobs. The user may ask for the job's output or status after the job has been stopped.

A future enhancement could be added to the server/API to remove a job from the map after it has been stopped.

#### Security

The client and API communicate via mTLS using TLS 1.3 as the minimum version. The following cipher suites are supported:

- TLS_AES_128_GCM_SHA256
- TLS_AES_256_GCM_SHA384
- TLS_CHACHA20_POLY1305_SHA256

Note: [Go does not support configuring supported cipher suites when using TLS 1.3](https://pkg.go.dev/crypto/tls#Config).

#### Authorization

The API treats the client's common name (CN) in the certificate as the user's identity.

A client/user may not interact with a job not created by them in any way. No querying, streaming, or stopping of another user's job is allowed.

Each handler may retrieve the user's identity by:

1. extracting the peer from the context
1. extracting the certificate from the peer's auth information
1. looking up the common name in the certificate

#### Protobuf

The proposed protobuf file for the client/server is as follows:

```proto
syntax = "proto3";

// ... options ...

package jobworker;

service JobWorker {
  rpc Start(JobStartRequest) returns (Job) {}
  rpc Query(Job) returns (JobStatus) {}
  rpc Stream(Job) returns (stream Output) {}
  rpc Stop(Job) returns (JobStopResponse) {}
}

message Job {
  string id = 1;
}

message JobStartRequest {
  double cpu = 1;
  int64 mem_bytes = 2;
  int64 io_bytes_per_second = 3;
  string command = 4;
  repeated string args = 5;
}

message JobStatus {
  string status = 1;
  int32 exit_code = 2;
  string exit_reason = 3;
}

message Output {
  bytes content = 1;
}

message JobStopResponse {}
```

### Library

A library written in Golang supports running an arbitrary Linux process in isolation with resource limits.

This library should know nothing about the client or API. It should be able to be used in any Golang program and treated as
a public library.

Jobs should be safe for concurrent use. Consider a mutex to handle concurrent changes to job's state such as stop being invoked
multiple times concurrently.

#### Process isolation

The process is isolated by:
- creating a new pid namespace to prevent a process from killing other processes running on the machine not created by the job
- creating a new network namespace to prevent all receiving or sending network traffic both on the local network and internet
- creating a new mount namespace which prevents modifying the host's mounts
   - a new mount namespace also enables mounting a new proc filesystem that prevents the process from even seeing other processes on the host (e.g. `ps -ef`)

#### Process resource limits

The process's resources are limited by configuring cgroups. The user is expected to provide CPU, memory, and IO limits. cgroup v2 will be used by the library.

#### Approximate Go doc

The following describes types and functions that may exist in a new package for the library.

##### type Job

`Job` holds internal information about a job such as an [exec.Cmd](https://pkg.go.dev/os/exec#Cmd)  and process output.

A `Job` should not be created directly. Instead, use the [New](#func-newconfig-jobconfig-job) function.

#### type JobConfig

```go
type JobConfig struct {
  JobExecutorPath  string
  CPU              float64
  MemBytes         int64
  IOBytesPerSecond int64
  Command          string
  Arguments        []string
}
```

`JobConfig` holds configuration for creating a new job.

All fields are required to be a non-zero value except `Arguments` to be able to successfully invoke [Job.Start](func-job-start-error).
`Job.Start` will return an error if any of the fields are zero values.

- `JobExecutorPath` is the path to the binary that will execute the provided `Command` and `Arguments`. It is expected that the binary will mount a new proc filesystem and then fork+execute the user's command with any arguments.
- `CPU` is a decimal representing an approximate number of CPU cores to limit the job to. For example, `0.5` would translate to half a CPU core. This is configured by setting `cpu.max` as `500000 1000000` in the cgroup for the process.
- `MemBytes` is the maximum amount of memory to be used by the job. This is configured by setting `memory.max` in the cgroup for the process using the same number provided.
- `IOBytesPerSecond` is the maximum read and write on the device mounted `/` is mounted on. This is configured by setting `io.max` in the cgroup for the process. For example, a `IOBytesPerSecond` of `1000000000` would be similar to `259:1 rbps=1000000000 wbps=1000000000 riops=max wiops=max` in the cgroup's `io.max` file.
- `Command` is the command to execute. For example, `/bin/bash`.
- `Arguments` are the arguments to pass to the command. For example, `[]string{"-c", "echo hello"}` would be provided to command and ultimately run similar to `/bin/bash -c "echo hello"`.

`JobExecutorPath` should handle `SIGTERM`. If `SIGTERM` is received, then `JobExecutorPath` should send `SIGTERM` to the process it's running (user's command).
`JobExecutorPath` should wait for the child process to complete before exiting.

##### func New(config JobConfig) *Job

`New` creates a new `Job`. No command is started until `Start` is invoked.

It is expected that the config's `JobExecutorPath` is a binary that will execute the provided `Command` and `Arguments` similar to:

```bash
<JobExecutorPath> run <Command> <Arguments>
```

It is expected that `JobExecutorPath` will mount a new proc filesystem and then fork+execute the user's command with any arguments.

> Note: `JobExecutorPath` will be a new binary entirely created from this design for the library to use. Eventually, the server's binary
> will be a compatible `JobExecutorPath` that the library can be instructed to use. This way to deploy the server requires only the one
> server binary instead of the server binary and the `JobExecutorPath` binary. The server can then provide `/proc/self/exe` (itself) to the library
> as `JobExecutorPath`.

The `JobExecutorPath` may seem odd, but this enables a "hook" to create and mount a new proc filesystem before forking + executing the desired command with any arguments.
This technique enables process execution within the newly created mount namespace to mount a new proc filesystem. Without the `JobExecutorPath`, if the user's command
was executed directly, then the user's command would still use the host's proc filesystem.

The library should provide an exported function(s) that can mount and unmount a new proc file system. The library should also provide an exported function
that can handle signals such as `SIGTERM` and send `SIGTERM` to the process it's running (user's command).

##### func (*Job) Start() error

`Start` creates a configured [exec.Cmd](https://pkg.go.dev/os/exec#Cmd).

`Start` will return an error if the `Job` was provided a `JobConfig` with zero values for any of the fields, except `Arguments`.

`Start` takes the following steps:

1. creates a new cgroup for the job such as `/sys/fs/cgroup/<uuid>`
1. enables subtrees to control resource limits by adding `+cpu +memory +io` to `/sys/fs/cgroup/<uuid>/cgroup.subtree_control` file
1. sets the CPU, memory, and IO limits based on the `JobConfig`
   - CPU is set by writing `500000 1000000` to `/sys/fs/cgroup/<uuid>/tasks/cpu.max`
   - memory is set by writing the provided `MemBytes` to `/sys/fs/cgroup/<uuid>/tasks/memory.max`
   - io is set by writing `259:1 rbps=<IOBytesPerSecond> wbps=<IOBytesPerSecond> riops=max wiops=max` to `/sys/fs/cgroup/<uuid>/tasks/io.max`
      - `259:1` is the major and minor number of the physical device that `/` is mounted on as an example. Need to look this up dynamically or provide it as a configuration option.
1. configures a new [exec.Cmd](https://pkg.go.dev/os/exec#Cmd)
   - sets clone flags for creating new mount, pid, and network namespaces
   - sets unshare flags for mount so that new mounts are not reflected in host mount namespace
   - sets the command to be `JobExecutorPath` (its process is explained below) with additional arguments to run `JobExecutorPath` with the user's provided `Command` and `Arguments`
   - sets [UseCgroupFD and CgroupFD](https://pkg.go.dev/syscall#SysProcAttr) to the cgroup's file descriptor such as `/sys/fs/cgroup/<uuid>/tasks`
1. runs the configured `exec.Cmd` in a new goroutine
   - `exec.Cmd` will fork and execute `JobExecutorPath`
   - does not wait for the command to complete
   - the goroutine will stop when the process completes or is killed

As described above, `Start`'s configured `exec.Cmd` will execute `jobExecutorPath`. It is expected that `jobExecutorPath` is a binary that will do the following:

1. mount a new proc filesystem to limit the process's view of the host's processes
1. execute the provided `command` and `arguments` using [exec.Cmd](https://pkg.go.dev/os/exec#Cmd), which forks and executes the command
   - This means that `jobExecutorPath` is PID 1, while the user's command is a child process of `jobExecutorPath`
   - This also means that killing `jobExecutorPath` will kill all other spawned processes in the pid namespace
   - In the future, if it's desired for the user's command to be PID 1, then `jobExecutorPath` should use [syscall.Exec](https://pkg.go.dev/syscall#Exec) for the user's command and arguments. And take careful steps to ensure signals are appropriately handled.

The `jobExecutorPath` will be executed similar to:

```bash
<jobExecutorPath> run <command> <arguments>
```

Finally, `Start` runs the configured `exec.Cmd` in a new goroutine. `Start` does not wait for the command to complete. The goroutine will stop when the process completes or is killed. Once the process is completed (regardless of success), then the cgroups will be removed.

It is an error to invoke `Start` on a `Job` that has already been started regardless of the job's status.

##### func (*Job) Status() JobStatus

`Status` returns the current status of the job. Refer to [JobStatus](#type-jobstatus) for more information.

##### func (*Job) Stream() OutputReader

`Stream` returns an `OutputReader`. An `OutputReader` is a type that implements [io.Reader](https://pkg.go.dev/io#Reader). OutputReader is described in [OutputReader](#outputreader).

`OutputReader` enables a user to read the combined output of the process's stdout and stderr as it runs. `OutputReader` is able to read the process's entire output from when it started to the current output.

It is okay to call `Stream` for a job that has already been completed.

It is okay to call `Stream` on a job that has not been started.

##### func (*Job) Stop() error

`Stop` will attempt to gracefully shutdown the process by sending a SIGTERM signal to the process via [exec.Cmd.Process.Signal](https://pkg.go.dev/os/exec#Cmd.Process).

It is expected that `JobExecutorPath` will handle SIGTERM and send SIGTERM to the process it's running (user's command).

If the process (`JobExecutorPath`) has not terminated after 10 seconds, then `Stop` will send a SIGKILL signal to the process via [exec.Cmd.Process.Kill](https://pkg.go.dev/os/exec#Cmd.Process).

Since the process (`JobExecutorPath`) is PID 1 in the new pid namespace, killing it will kill all other spawned process in the pid namespace.

`Stop` will do nothing if the process has already completed or been killed.

##### type JobStatus

```go
type JobStatus struct {
  State     string
  ExitCode  int
  ExitReason string
}
```

The `JobStatus` holds information about the job's status such as state, the exit code, and the exit reason.

Exit code will be -1 if not in a completed state. Otherwise, it will be the process's exit code if process invoked exit. If the
process was killed by a signal, then the exit code will be -1.

Exit reason is populated with the error message returned by `exec.Cmd.Run` if the process failed to run.

### Streaming considerations

The API needs to be able to stream the process's output to multiple clients concurrently. Each client should receive the output
from the start of the process regardless of when the client began streaming.

The Job's `Stream` function will return an `OutputReader` of the process's output. The Job needs to be able to identify in a performant
manner when new output has been written and send that to each client.

#### Proposed solution

**Assume that the server has enough memory to hold all running jobs' stdout and stderr in memory.**

Two more types are being proposed.

1. `Output` to handle concurrent writes (a process's stdout and stderr) and reads (multiple clients reading/streaming the output).

1. `OutputReader` to handle reading the process's output as it's written to `Output`.

`Output` and `OutputReader` should know nothing about the library, API, or client. These new types should not be treated as public and should exist
within an `internal` directory to prevent any imports outside of this repository.

##### Output

Create a new type named `Output` that implements [io.Writer](https://pkg.go.dev/io#Writer), [io.ReaderAt](https://pkg.go.dev/io#ReaderAt), and [io.Closer](https://pkg.go.dev/io#Closer). This new type should be able to support multiple writers (a process's stdout and stderr) and multiple readers (multiple clients reading/streaming the output).

A single `Output` will be provided as the job's stdout and stderr. `Output` will then be closed by the job once the process has completed or been killed.

`Output` can store all process output in a `[]byte`. Consider a mutex to handle concurrent read/writes.

#### OutputReader

The Job's `Stream` function will create a new `OutputReader` per invocation that is provided the Job's `Output`.

`OutputReader` will implement [io.Reader](https://pkg.go.dev/io#Reader). `OutputReader`'s `Read` will invoke the underlying `Output`'s `ReadAt` to read the next bytes.

The above handles reading existing output, but this solution also needs to handle new output being written to the `Output` while clients are streaming.

`OutputReader` does not need to safe for concurrent usage. Job's `Stream` will create a new `OutputReader` per invocation. It is up to the user of `OutputReader` to handle concurrent usage, if needed.

##### Enable OutputReader to Detect changes to Output's content

To performantly know when new bytes have been added to `Output`'s underlying content, consider using a
[sync.Cond](https://pkg.go.dev/sync#Cond) to broadcast when new bytes have been written to `Output`'s content.

`Output` should broadcast when new bytes have been written or the `Output` has been closed.

`OutputReader`'s `Read` should wait for the broadcast to read new bytes until receiving an [io.EOF](https://pkg.go.dev/io#pkg-variables).

### Test Plan

Create a few end-to-end tests to verify some behaviors like:

1. Demonstrate starting, checking the job's status, and stopping a job.
1. Demonstrate starting a job, streaming the job output, and validating the stream completes when the process completes.
1. Demonstrate a user may not interact with another user's job.
1. Demonstrate a job may not make network requests.
1. Demonstrate cgroups' limits are being enforced by running commands such as:
   - `dd if=/dev/zero of=/tmp/test bs=512M count=1` to verify io.max (`dd` outputs rough approximations of read and write speeds)
   - `sha1sum /dev/random` to verify cpu.limit (while it's running, can use something like `ps -p $(pgrep sha1sum) -o %cpu` to verify CPU usage)
