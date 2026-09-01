package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestJobStore(t *testing.T) {
	t.Parallel()

	store := NewJobStore()

	// 1. Create job
	job := store.Create("data_export", map[string]any{"user": "u1"})
	if job.ID == "" || job.Status != JobStatusPending {
		t.Fatalf("expected pending job with ID, got %v", job)
	}

	// 2. Get job
	retrieved, ok := store.Get(job.ID)
	if !ok || retrieved.ID != job.ID {
		t.Fatalf("failed to retrieve created job: %v", retrieved)
	}

	// 3. Update progress
	store.UpdateProgress(job.ID, 45, JobStatusProcessing)
	retrieved, _ = store.Get(job.ID)
	if retrieved.Progress != 45 || retrieved.Status != JobStatusProcessing {
		t.Errorf("expected 45%% progress in processing status, got %v", retrieved)
	}

	// 4. Complete job
	store.Complete(job.ID, "/downloads/export_123.csv")
	retrieved, _ = store.Get(job.ID)
	if retrieved.Status != JobStatusCompleted || retrieved.Progress != 100 || retrieved.ResultURL == "" {
		t.Errorf("expected completed job with result URL, got %v", retrieved)
	}

	// 5. Fail another job
	failedJob := store.Create("report_gen", nil)
	store.Fail(failedJob.ID, "out of memory")
	retrieved, _ = store.Get(failedJob.ID)
	if retrieved.Status != JobStatusFailed || retrieved.Error != "out of memory" {
		t.Errorf("expected failed job with error message, got %v", retrieved)
	}

	// 6. Cancel a pending job
	cancelJob := store.Create("bulk_email", nil)
	if !store.Cancel(cancelJob.ID) {
		t.Errorf("expected cancel to return true for in-flight job")
	}
	retrieved, _ = store.Get(cancelJob.ID)
	if retrieved.Status != JobStatusCancelled {
		t.Errorf("expected cancelled status, got %v", retrieved)
	}
}

func TestJobHandlerEndpoints(t *testing.T) {
	t.Parallel()

	store := NewJobStore()
	handler := NewJobHandler(store)

	r := chi.NewRouter()
	r.Post("/jobs/export", handler.CreateExportJob)
	r.Get("/jobs/{id}", handler.GetJob)
	r.Delete("/jobs/{id}", handler.CancelJob)

	t.Run("CreateExportJob returns 202 Accepted with Location header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/jobs/export", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusAccepted {
			t.Fatalf("expected status 202 Accepted, got %d", w.Code)
		}

		location := w.Header().Get("Location")
		if location == "" || !strings.HasPrefix(location, "/jobs/") {
			t.Fatalf("expected Location header starting with /jobs/, got %s", location)
		}
		if w.Header().Get("Retry-After") != "2" {
			t.Errorf("expected Retry-After 2, got %s", w.Header().Get("Retry-After"))
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response JSON: %v", err)
		}
		if _, ok := resp["_links"]; !ok {
			t.Error("expected _links in 202 Accepted response body")
		}
	})

	t.Run("GetJob returns 200 with job details", func(t *testing.T) {
		job := store.Create("test_job", nil)

		req := httptest.NewRequest(http.MethodGet, "/jobs/"+job.ID, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var retrieved Job
		if err := json.NewDecoder(w.Body).Decode(&retrieved); err != nil {
			t.Fatalf("failed to decode job: %v", err)
		}
		if retrieved.ID != job.ID {
			t.Errorf("expected job ID %s, got %s", job.ID, retrieved.ID)
		}
	})

	t.Run("GetJob returns 404 for non-existent job", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/jobs/non_existent_id", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", w.Code)
		}
	})

	t.Run("CancelJob returns 204 No Content for active job", func(t *testing.T) {
		job := store.Create("cancellable_job", nil)

		req := httptest.NewRequest(http.MethodDelete, "/jobs/"+job.ID, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected status 204 No Content on cancel, got %d", w.Code)
		}
	})
}

func TestRunBackgroundWorkerPool(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	tasks := make(chan func(), 5)

	doneCh := make(chan struct{})
	go func() {
		RunBackgroundWorkerPool(ctx, 2, tasks)
		close(doneCh)
	}()

	var executedCount int
	taskDone := make(chan bool, 2)
	for range 2 {
		tasks <- func() {
			taskDone <- true
		}
	}

	for range 2 {
		<-taskDone
		executedCount++
	}

	if executedCount != 2 {
		t.Errorf("expected 2 tasks executed, got %d", executedCount)
	}

	cancel()
	select {
	case <-doneCh:
		// Pool stopped cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("worker pool did not stop within timeout")
	}
}
