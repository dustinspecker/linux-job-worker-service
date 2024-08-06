# Linux Job Worker Service

> A library to run jobs on Linux 64-bit systems in an isolated manner.

## Purpose

This library is intended to be used to run arbitrary commands on Linux 64-bit systems. To help prevent disruptions
to other processes running on the system, the job is run in an isolated manner. This isolation includes:

- new PID namespace to prevent killing other processes on the host
- new mount namespace to mount a new proc filesystem so that the job can't see other processes on the host
- new network namespace to prevent the job from accessing the local network and internet
- configurable CPU, memory, and IO limits to throttle the job using cgroups v2

## Install

```bash
go get github.com/dustinspecker/linux-job-worker-service
```

## Usage

An example to run a job that echoes "Hello, World!":

```go
package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/dustinspecker/linux-job-worker-service/pkg/executor"
	"github.com/dustinspecker/linux-job-worker-service/pkg/job"
)

func main() {
	if len(os.Args) > 3 && os.Args[1] == "run" {
		err := executor.Run(os.Args[2], os.Args[3:])
		if err != nil {
			log.Fatalf("error running command: %v", err)
		}
		return
	}

	// create a job config
	config := job.Config{
		RootPhysicalDeviceMajMin: "259:1",          // example of how to find this value is in the Development section
		JobExecutorPath:          "/proc/self/exe", // this will re-run the same executable but as `<exe> run command arguments
		Command:                  "echo",
		Arguments:                []string{"Hello, World!"},
		IOInBytes:                10_000_000,  // 10 MB read/write limit on the physical RootDeviceMajorMinor device
		MemoryInBytes:            100_000_000, // 100 MB memory limit for the command to use
		CPU:                      0.5,         // 50% of a single CPU core for the command to be throttled to
	}

	// create job
	echoJob := job.New(config)

	// start the job
	if err := echoJob.Start(); err != nil {
		panic(err)
	}

	// for long-lived jobs, they may optionally be stopped by running:
	// if err := job.Stop(); err != nil {
	//     panic(err)
	// }

	// collect the job's combined stdout and stderr until the job completes
	output, err := io.ReadAll(echoJob.Stream())
	if err != nil {
		panic(err)
	}

	fmt.Println(string(output))

	// get the job's exit code
	status := echoJob.Status()
	fmt.Printf("exit code: %d", status.ExitCode)
}
```

Note: it is important that the application supports running `executor.Run` when the program is executed
as `<exe> run command arguments`. The job library will re-execute the program with `run` when the job
configuration is told to use `/proc/self/exe` as the `JobExecutorPath`.

## Development

1. Install [Go](https://golang.org/doc/install)
1. Install [protoc and Go plugins](https://grpc.io/docs/languages/go/quickstart/)
1. Clone repository
1. Run linting via (be sure to install [golangci-lint 1.59.1](https://github.com/golangci/golangci-lint/releases/tag/v1.59.1):

   ```bash
   make lint
   ```

1. Run unit tests via:

   ```bash
   make test-unit
   ```

1. Build binaries via:

   ```bash
   make build
   ```

1. Find the physical device Major/Minor of where `/` is mounted by running:

   ```bash
   lsblk
   ```

   example output:

   ```
   NAME        MAJ:MIN RM   SIZE RO TYPE MOUNTPOINTS
   loop0         7:0    0     4K  1 loop /snap/bare/5
   loop1         7:1    0  63.9M  1 loop /snap/core20/2318
   loop2         7:2    0  74.2M  1 loop /snap/core22/1380
   loop3         7:3    0 268.9M  1 loop /snap/firefox/4630
   loop4         7:4    0 268.4M  1 loop /snap/firefox/4650
   loop5         7:5    0  10.7M  1 loop /snap/firmware-updater/127
   loop6         7:6    0 349.7M  1 loop /snap/gnome-3-38-2004/143
   loop7         7:7    0 505.1M  1 loop /snap/gnome-42-2204/176
   loop8         7:8    0  91.7M  1 loop /snap/gtk-common-themes/1535
   loop9         7:9    0  10.4M  1 loop /snap/snap-store/1147
   loop10        7:10   0  10.5M  1 loop /snap/snap-store/1173
   loop11        7:11   0  38.7M  1 loop /snap/snapd/21465
   loop12        7:12   0  38.8M  1 loop /snap/snapd/21759
   loop13        7:13   0   476K  1 loop /snap/snapd-desktop-integration/157
   loop14        7:14   0 183.7M  1 loop /snap/spotify/77
   loop15        7:15   0 181.8M  1 loop /snap/spotify/78
   sr0          11:0    1  86.3M  0 rom
   nvme0n1     259:0    0 931.5G  0 disk
   ├─nvme0n1p1 259:2    0   100M  0 part
   ├─nvme0n1p2 259:3    0    16M  0 part
   ├─nvme0n1p3 259:4    0 930.7G  0 part
   └─nvme0n1p4 259:5    0   749M  0 part
   nvme1n1     259:1    0 931.5G  0 disk
   ├─nvme1n1p1 259:6    0     1G  0 part /boot/efi
   └─nvme1n1p2 259:7    0 930.5G  0 part /var/snap/firefox/common/host-hunspell
                                         /
   nvme2n1     259:8    0   1.8T  0 disk
   ├─nvme2n1p1 259:9    0    16M  0 part
   └─nvme2n1p2 259:10   0   1.8T  0 part
   ```

   Find the MAJ:MIN of the disk type that `/` is mounted on. In the above output, `259:1` is the MAJ:MIN of the disk that `/` is mounted on.

1. Generate certificates via:

   ```bash
   make certs
   ```

1. Run integration tests via

   ```bash
   sudo env PATH=$PATH ROOT_DEVICE_MAJ_MIN=259:1 make test-integration
   ```

   Note: `sudo` is required since the integration tests create new namespaces and control groups (cgroups).
