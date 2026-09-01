package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// JobStatus represents the current state of an asynchronous long-running task.
type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusCancelled  JobStatus = "cancelled"
)

// Job represents a background asynchronous task tracked by the API.
type Job struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Status      JobStatus      `json:"status"`
	Progress    int            `json:"progress"` // 0 - 100
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	CompletedAt *time.Time     `json:"completedAt,omitempty"`
	ResultURL   string         `json:"resultUrl,omitempty"`
	Error       string         `json:"error,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// JobStore manages background job states in memory (in production, use Redis or a relational DB).
type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewJobStore creates a new in-memory job store.
func NewJobStore() *JobStore {
	return &JobStore{jobs: make(map[string]*Job)}
}

func (s *JobStore) Create(jobType string, metadata map[string]any) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := "job_" + hex.EncodeToString(b)

	now := time.Now().UTC()
	job := &Job{
		ID:        id,
		Type:      jobType,
		Status:    JobStatusPending,
		Progress:  0,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  metadata,
	}

	s.jobs[id] = job
	return job
}

func (s *JobStore) Get(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	// Return copy
	jobCopy := *j
	return &jobCopy, true
}

func (s *JobStore) UpdateProgress(id string, progress int, status JobStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if j, ok := s.jobs[id]; ok {
		j.Progress = progress
		j.Status = status
		j.UpdatedAt = time.Now().UTC()
	}
}

func (s *JobStore) Complete(id string, resultURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if j, ok := s.jobs[id]; ok {
		now := time.Now().UTC()
		j.Status = JobStatusCompleted
		j.Progress = 100
		j.ResultURL = resultURL
		j.UpdatedAt = now
		j.CompletedAt = &now
	}
}

func (s *JobStore) Fail(id string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if j, ok := s.jobs[id]; ok {
		now := time.Now().UTC()
		j.Status = JobStatusFailed
		j.Error = errMsg
		j.UpdatedAt = now
		j.CompletedAt = &now
	}
}

func (s *JobStore) Cancel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if j, ok := s.jobs[id]; ok {
		if j.Status == JobStatusCompleted || j.Status == JobStatusFailed {
			return false
		}
		now := time.Now().UTC()
		j.Status = JobStatusCancelled
		j.UpdatedAt = now
		j.CompletedAt = &now
		return true
	}
	return false
}

// JobHandler implements asynchronous task dispatching and polling endpoints.
type JobHandler struct {
	store *JobStore
}

// NewJobHandler creates a new job controller.
func NewJobHandler(store *JobStore) *JobHandler {
	return &JobHandler{store: store}
}

// CreateExportJob handles POST /jobs/export — demonstrates 202 Accepted asynchronous pattern.
func (h *JobHandler) CreateExportJob(w http.ResponseWriter, r *http.Request) {
	job := h.store.Create("orders_export_csv", map[string]any{
		"requestedBy": GetRequestID(r.Context()),
	})

	// Simulate background processing worker
	go func(jobID string) {
		time.Sleep(100 * time.Millisecond)
		h.store.UpdateProgress(jobID, 50, JobStatusProcessing)
		time.Sleep(100 * time.Millisecond)
		h.store.Complete(jobID, fmt.Sprintf("/api/v1/downloads/%s.csv", jobID))
	}(job.ID)

	location := fmt.Sprintf("/jobs/%s", job.ID)
	w.Header().Set("Location", location)
	w.Header().Set("Retry-After", "2")
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job":     job,
		"message": "Asynchronous job accepted for processing. Check status at Location URL.",
		"_links": map[string]Link{
			"status": {Href: location, Method: http.MethodGet},
			"cancel": {Href: location, Method: http.MethodDelete},
		},
	})
}

// GetJob handles GET /jobs/{id} — demonstrates polling status endpoint with Retry-After.
func (h *JobHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, found := h.store.Get(id)
	if !found {
		NotFound(w, r, fmt.Sprintf("Job '%s' not found.", id))
		return
	}

	if job.Status == JobStatusPending || job.Status == JobStatusProcessing {
		w.Header().Set("Retry-After", "2")
	}

	writeJSON(w, http.StatusOK, job)
}

// CancelJob handles DELETE /jobs/{id} — cancels an in-flight job.
func (h *JobHandler) CancelJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cancelled := h.store.Cancel(id)
	if !cancelled {
		Conflict(w, r, fmt.Sprintf("Job '%s' cannot be cancelled (either not found or already finished).", id))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RunBackgroundWorkerPool starts a simple pool of background task processors.
func RunBackgroundWorkerPool(ctx context.Context, numWorkers int, tasks <-chan func()) {
	var wg sync.WaitGroup
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case task, ok := <-tasks:
					if !ok {
						return
					}
					task()
				}
			}
		}()
	}
	<-ctx.Done()
	wg.Wait()
}
