package oblodai

import (
	"context"
	"testing"
	"time"
)

// Long-running operations: a batch or a document export is accepted at once and finishes later;
// a Job polls it until its status is terminal.

func TestTheLROTableNamesRealOperations(t *testing.T) {
	for create, poll := range LRO {
		if _, ok := Routes[create]; !ok {
			t.Errorf("LRO: no operation %s", create)
		}
		p, ok := Polls[poll]
		if _, route := Routes[poll]; !ok || !route {
			t.Errorf("LRO: %s polls %s, which is not a route with a Poll", create, poll)
		}
		if p.Download != "" && !Routes[p.Download].Bare {
			t.Errorf("Polls: %s downloads through %s, which is not a file route", poll, p.Download)
		}
	}
}

func recordSleeps(pauses *[]time.Duration) Option {
	return withSleep(func(_ context.Context, d time.Duration) error {
		*pauses = append(*pauses, d)
		return nil
	})
}

func TestABatchJobIsPolledUntilItsStatusIsTerminal(t *testing.T) {
	api := newFakeAPI(t,
		ok(map[string]any{"batch_id": "b1", "kind": "payout", "count": 2, "status": "pending"}),
		ok(map[string]any{"batch_id": "b1", "status": "processing", "total": 2}),
		ok(map[string]any{"batch_id": "b1", "status": "completed", "total": 2, "succeeded": 2}),
	)
	var pauses []time.Duration
	client := api.client(recordSleeps(&pauses))
	ctx := context.Background()
	accepted, err := client.Batches.CreatePayout(ctx, &PayoutBatchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	job := client.BatchJob(accepted.BatchID)
	if job.ID != "b1" {
		t.Fatalf("job id %q", job.ID)
	}
	info, err := job.Wait(ctx, WithPollInterval(3*time.Second))
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if info.Status != BatchStatusCompleted || info.Succeeded != 2 {
		t.Fatalf("final answer %+v", info)
	}
	if api.count() != 3 || api.at(1).path != "/v1/batch/info" || api.at(1).jsonBody(t)["batch_id"] != "b1" {
		t.Fatalf("polls: %d requests, %s %v", api.count(), api.at(1).path, api.at(1).jsonBody(t))
	}
	if len(pauses) != 1 || pauses[0] != 3*time.Second {
		t.Fatalf("pauses %v: one interval between two polls", pauses)
	}
	if api.at(1).header.Get(HeaderIdempotencyKey) != "" {
		t.Fatal("a poll is a read: no idempotency key")
	}
}

func TestADocumentJobEndsFailedWithoutAnErrorAndDownloads(t *testing.T) {
	api := newFakeAPI(t,
		ok(map[string]any{"job_id": "j1", "status": "failed", "error": map[string]any{"code": "documents.too_large"}}),
		step{status: 200, body: "%PDF", headers: map[string]string{"Content-Type": "application/pdf"}},
	)
	client := api.client()
	ctx := context.Background()
	view, err := client.DocumentJob("j1").Wait(ctx)
	if err != nil {
		t.Fatalf("a terminal status is an answer, not an error: %v", err)
	}
	if view.Status != DocumentJobStatusFailed || view.Error == nil {
		t.Fatalf("view %+v", view)
	}
	file, err := client.DocumentJob("j1").Download(ctx)
	if err != nil || string(file.Bytes) != "%PDF" {
		t.Fatalf("Download: %v", err)
	}
	if got := api.at(1); got.path != "/v1/documents/jobs/file" || got.rawQuery != "job_id=j1" {
		t.Fatalf("download request %s?%s", got.path, got.rawQuery)
	}
	if _, err := client.BatchJob("b1").Download(ctx); !IsConfig(err) {
		t.Fatalf("a batch makes no file: %v", err)
	}
}

func TestWaitGivesUpAfterItsTimeout(t *testing.T) {
	processing := ok(map[string]any{"batch_id": "b1", "status": "processing"})
	api := newFakeAPI(t, processing, processing, processing, processing)
	var pauses []time.Duration
	client := api.client(recordSleeps(&pauses))
	_, err := client.BatchJob("b1").Wait(context.Background(), WithPollInterval(time.Millisecond), WithWaitTimeout(0))
	requireCode(t, err, CodeWaitTimeout)
	if api.count() != 1 {
		t.Fatalf("a zero timeout polls once, saw %d", api.count())
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.BatchJob("b1").Wait(cancelled)
	if !IsTransport(err) {
		t.Fatalf("a cancelled wait: %v", err)
	}
}

func TestJobForFollowsAnyLongRunningCall(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{"job_id": "j2", "status": "done"}))
	client := api.client()
	job, err := JobFor[DocumentJobView](client, "createDocumentJob", "j2")
	if err != nil {
		t.Fatal(err)
	}
	view, err := job.Wait(context.Background())
	if err != nil || view.JobID != "j2" {
		t.Fatalf("%v %+v", err, view)
	}
	if _, err := JobFor[DocumentJobView](client, "createPayment", "x"); !IsConfig(err) {
		t.Fatalf("createPayment is not long-running: %v", err)
	}
}
