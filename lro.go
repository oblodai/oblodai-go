package oblodai

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"time"
)

// Long-running operations. A batch or a document export is accepted at once and finishes later:
// the create call answers with the job's id, and a Job polls the operation that reports on it
// until the status is terminal:
//
//	accepted, err := client.Batches.CreatePayout(ctx, params)
//	info, err := client.BatchJob(accepted.BatchID).Wait(ctx)
//	// info.Status is completed or stopped; info.Items say how each payout went
//
// Which operations are long-running, and how each is followed, is a fact of the contract
// (x-sdk-poll): the generator writes it into the LRO and Polls tables (zz_generated_facts.go).

// Poll is how the jobs of one poll operation are followed (a value of Polls).
type Poll struct {
	// IDField is the job id's name in the create answer; the poll (and the download) take it
	// under the same name.
	IDField string
	// StatusField is the field of the poll's answer that holds the job's status.
	StatusField string
	// Terminal are the statuses after which the job no longer changes.
	Terminal []string
	// Download is the operation that returns the finished job's file, if the job makes one.
	Download string
}

// Job follows one long-running operation. T is the poll's answer.
type Job[T any] struct {
	// ID is the job's id (batch_id, job_id).
	ID string

	r        Requester
	sleep    func(context.Context, time.Duration) error
	poll     RouteSpec
	follow   Poll
	download *RouteSpec
	opts     []RequestOption
	err      error // the job cannot be followed (jobOf found no operation)
}

// BatchJob follows a batch — payment, payout, refund or transfer — by its batch_id.
func (c *Client) BatchJob(batchID string, opts ...RequestOption) *Job[BatchInfoResponse] {
	return jobOf[BatchInfoResponse](c, batchID, opts)
}

// DocumentJob follows a document export by its job_id; Download fetches the file once it is done.
func (c *Client) DocumentJob(jobID string, opts ...RequestOption) *Job[DocumentJobView] {
	return jobOf[DocumentJobView](c, jobID, opts)
}

// jobOf follows a job by the model its poll answers with, so the helpers above name no operation:
// the long-running operations whose poll answers T come from the generated LRO and Polls. When the
// contract polls no such operation, the job's Poll, Wait and Download report sdk.bad_config.
func jobOf[T any](c *Client, id string, opts []RequestOption) *Job[T] {
	for _, create := range slices.Sorted(maps.Keys(LRO)) {
		if job, err := JobFor[T](c, create, id, opts...); err == nil {
			return job
		}
	}
	return &Job[T]{ID: id, err: newConfigError(CodeBadConfig, fmt.Sprintf("no long-running operation is polled with %T", new(T)), "")}
}

// JobFor follows the job that the long-running operation createOperationID (a key of LRO) created
// with id; T is what its poll answers (the poll's generated model). The options apply to every
// poll, except that a poll never carries an idempotency key and gets its own request id.
func JobFor[T any](c *Client, createOperationID, id string, opts ...RequestOption) (*Job[T], error) {
	pollOp, ok := LRO[createOperationID]
	if !ok {
		return nil, newConfigError(CodeBadConfig, createOperationID+" is not a long-running operation (see LRO)", "")
	}
	if _, ok := pollModels[pollOp]().(*T); !ok {
		return nil, newConfigError(CodeBadConfig, fmt.Sprintf("%s answers with %T, not %T", pollOp, pollModels[pollOp](), new(T)), "")
	}
	follow := Polls[pollOp]
	job := &Job[T]{
		ID:     id,
		r:      c.transport,
		sleep:  c.transport.sleep,
		poll:   Routes[pollOp],
		follow: follow,
		opts: append(slices.Clone(opts), func(o *callOptions) {
			o.idempotencyKey, o.requestID = "", ""
		}),
	}
	if follow.Download != "" {
		download := Routes[follow.Download]
		job.download = &download
	}
	return job, nil
}

// Poll asks once how the job is doing.
func (j *Job[T]) Poll(ctx context.Context) (*T, error) {
	if j.err != nil {
		return nil, j.err
	}
	return doJSON[T](ctx, j.r, Call{Route: j.poll, Body: map[string]string{j.follow.IDField: j.ID}}, j.opts)
}

// Wait polls until the job's status is terminal (Poll.Terminal) and returns that answer. A
// terminal status is returned, not raised: a failed job is inspected like a finished one. It
// gives up with sdk.wait_timeout after the wait timeout (5 minutes unless WithWaitTimeout), and
// with transport.aborted when ctx ends first.
func (j *Job[T]) Wait(ctx context.Context, opts ...WaitOption) (*T, error) {
	w := waitOptions{interval: 2 * time.Second, timeout: 5 * time.Minute}
	for _, opt := range opts {
		opt(&w)
	}
	sleep := j.sleep
	if sleep == nil {
		sleep = sleepContext
	}
	deadline := time.Now().Add(w.timeout)
	for {
		answer, err := j.Poll(ctx)
		if err != nil {
			return nil, err
		}
		status := statusOf(answer, j.follow.StatusField)
		if slices.Contains(j.follow.Terminal, status) {
			return answer, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, &Error{Kind: KindTransport, Code: CodeWaitTimeout, Retryable: true,
				Message: fmt.Sprintf("job %s is still %s after %s", j.ID, orUnfinished(status), w.timeout)}
		}
		if err := sleep(ctx, min(w.interval, remaining)); err != nil {
			return nil, newTransportError(CodeTransportAborted, "the wait for job "+j.ID+" was cancelled", err)
		}
	}
}

// Download fetches the finished job's file (document jobs only).
func (j *Job[T]) Download(ctx context.Context) (*FileResult, error) {
	if j.err != nil {
		return nil, j.err
	}
	if j.download == nil {
		return nil, newConfigError(CodeBadConfig, "job "+j.ID+" produces no file to download", "")
	}
	return doFile(ctx, j.r, Call{Route: *j.download, Query: url.Values{j.follow.IDField: {j.ID}}}, j.opts)
}

// WaitOption tunes Job.Wait.
type WaitOption func(*waitOptions)

type waitOptions struct {
	interval, timeout time.Duration
}

// WithPollInterval sets the pause between polls (default 2 s).
func WithPollInterval(d time.Duration) WaitOption {
	return func(w *waitOptions) {
		if d > 0 {
			w.interval = d
		}
	}
}

// WithWaitTimeout sets how long Wait keeps polling (default 5 minutes); 0 polls once.
func WithWaitTimeout(d time.Duration) WaitOption {
	return func(w *waitOptions) {
		if d >= 0 {
			w.timeout = d
		}
	}
}

// statusOf reads the status field of a poll answer through its JSON form.
func statusOf(answer any, field string) string {
	encoded, err := json.Marshal(answer)
	if err != nil {
		return ""
	}
	var head map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &head)
	var status string
	_ = json.Unmarshal(head[field], &status)
	return status
}

func orUnfinished(status string) string {
	if status == "" {
		return "unfinished"
	}
	return status
}
