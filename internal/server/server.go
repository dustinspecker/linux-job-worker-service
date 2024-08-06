package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/dustinspecker/linux-job-worker-service/internal/proto"
	"github.com/dustinspecker/linux-job-worker-service/internal/tls"
	"github.com/dustinspecker/linux-job-worker-service/pkg/job"
	"github.com/google/uuid"
)

var (
	ErrJobNotFound      = errors.New("job not found")
	ErrUserUnauthorized = errors.New("user is not authorized to access job")
)

// userJob holds the user who started the job and the job itself.
type userJob struct {
	user string
	job  *job.Job
}

// JobWorkerServer is an implementation of the proto.JobWorkerServer interface.
// JobWorkerServer should be created with NewJobWorkerServer, not directly.
type JobWorkerServer struct {
	proto.UnimplementedJobWorkerServer

	userJobs                 map[string]userJob
	mutex                    sync.Mutex
	rootPhysicalDeviceMajMin string
}

// NewJobWorkerServer creates a new JobWorkerServer.
func NewJobWorkerServer(rootPhysicalDeviceMajMin string) *JobWorkerServer {
	return &JobWorkerServer{
		userJobs:                 make(map[string]userJob),
		rootPhysicalDeviceMajMin: rootPhysicalDeviceMajMin,
	}
}

// Start creates a new job for the user and starts the job.
func (s *JobWorkerServer) Start(ctx context.Context, request *proto.JobStartRequest) (*proto.Job, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	user, err := tls.GetUserFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user from certificate: %w", err)
	}

	config := job.Config{
		RootPhysicalDeviceMajMin: s.rootPhysicalDeviceMajMin,
		// server implements job-executor so it can be re-executed to make deploying the server easier (a single binary)
		JobExecutorPath: "/proc/self/exe",
		CPU:             request.GetCpu(),
		IOInBytes:       request.GetIoBytesPerSecond(),
		MemoryInBytes:   request.GetMemBytes(),
		Command:         request.GetCommand(),
		Arguments:       request.GetArgs(),
	}

	newJob := job.New(config)
	jobID := uuid.New().String()
	s.userJobs[jobID] = userJob{
		user: user,
		job:  newJob,
	}

	response := proto.Job{
		Id: jobID,
	}

	if err := newJob.Start(); err != nil {
		return &response, fmt.Errorf("error starting job: %w", err)
	}

	return &response, nil
}

// Query returns the status of the job. Users may not query jobs started by other users.
func (s *JobWorkerServer) Query(ctx context.Context, protoJob *proto.Job) (*proto.JobStatus, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	jobID := protoJob.GetId()

	job, ok := s.userJobs[jobID]
	if !ok {
		return nil, ErrJobNotFound
	}

	user, err := tls.GetUserFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user from certificate: %w", err)
	}

	if user != job.user {
		// TODO: consider returning Not Found instead of Permission Denied to hide job existence
		return nil, ErrUserUnauthorized
	}

	jobStatus := job.job.Status()

	var exitReason string
	if jobStatus.ExitReason != nil {
		exitReason = jobStatus.ExitReason.Error()
	}

	return &proto.JobStatus{
		Status:     string(jobStatus.State),
		ExitCode:   int32(jobStatus.ExitCode),
		ExitReason: exitReason,
	}, nil
}

// Stream streams the output of the job. Users may not stream jobs started by other users.
func (s *JobWorkerServer) Stream(protoJob *proto.Job, stream proto.JobWorker_StreamServer) error {
	s.mutex.Lock()

	jobID := protoJob.GetId()

	job, ok := s.userJobs[jobID]
	if !ok {
		s.mutex.Unlock()

		return ErrJobNotFound
	}

	user, err := tls.GetUserFromContext(stream.Context())
	if err != nil {
		s.mutex.Unlock()

		return fmt.Errorf("failed to get user from certificate: %w", err)
	}

	if user != job.user {
		s.mutex.Unlock()

		// TODO: consider returning Not Found instead of Permission Denied to hide job existence
		return ErrUserUnauthorized
	}

	jobOutput := job.job.Stream()
	s.mutex.Unlock()

	for {
		buffer := make([]byte, 1024)

		bytesRead, err := jobOutput.Read(buffer)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return fmt.Errorf("error reading job output: %w", err)
			}

			err = stream.Send(&proto.Output{
				Content: buffer[:bytesRead],
			})
			if err != nil {
				return fmt.Errorf("error sending job output: %w", err)
			}

			break
		}

		err = stream.Send(&proto.Output{
			Content: buffer[:bytesRead],
		})
		if err != nil {
			return fmt.Errorf("error sending job output: %w", err)
		}
	}

	return nil
}

// Stop stops the job. Users may not stop jobs started by other users.
func (s *JobWorkerServer) Stop(ctx context.Context, protoJob *proto.Job) (*proto.JobStopResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	jobID := protoJob.GetId()

	job, ok := s.userJobs[jobID]
	if !ok {
		return nil, ErrJobNotFound
	}

	user, err := tls.GetUserFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user from certificate: %w", err)
	}

	if user != job.user {
		// TODO: consider returning Not Found instead of Permission Denied to hide job existence
		return nil, ErrUserUnauthorized
	}

	if err := job.job.Stop(); err != nil {
		return nil, fmt.Errorf("error stopping job: %w", err)
	}

	return &proto.JobStopResponse{}, nil
}
