package main

import (
	"log"
	"os"

	"github.com/dustinspecker/linux-job-worker-service/pkg/executor"
)

// job-executor satisfies the requirements of JobExecutorPath binary as described
// in the [rfd](rfd/0001-linux-job-worker-service.md).
// job-executor is used by integration tests.
// Refer to the readme.md for how applications importing this package can re-execute
// themselves instead of using job-executor.
func main() {
	if len(os.Args) > 3 && os.Args[1] == "run" {
		err := executor.Run(os.Args[2], os.Args[3:])
		if err != nil {
			log.Fatalf("error running command: %v", err)
		}

		return
	}

	log.Fatal("usage: job-executor run <command> <arguments>")
}
