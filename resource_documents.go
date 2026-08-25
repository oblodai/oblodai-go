package oblodai

import (
	"context"
	"net/url"
	"strconv"
)

// DocumentsService downloads generated documents. Every method returns the bytes; a large range
// goes through an asynchronous job instead (CreateJob, JobInfo, JobFile). Payment key, except the
// signed public download.
type DocumentsService struct{ c *Client }

// DocumentQuery is the common query of every document: the language it is rendered in.
type DocumentQuery struct {
	// Lang is a two-letter language code; 41 are supported. Defaults to the merchant's language.
	Lang string
}

// FormatQuery adds the output format where the document offers a choice.
type FormatQuery struct {
	Lang string
	// Format is "pdf" (default) or "csv".
	Format string
}

// PeriodQuery adds the reporting period.
type PeriodQuery struct {
	Lang   string
	Format string
	// From and To are YYYY-MM-DD dates.
	From string
	To   string
}

// DownloadQuery is the signed-link query of a public document: the exp and sig a document_url
// carries.
type DownloadQuery struct {
	Lang string
	// Exp is the unix second the link expires at.
	Exp int64
	// Sig is the link signature.
	Sig string
}

// CreateJob queues a large report (POST /v1/documents/jobs); poll JobInfo, then JobFile.
func (s *DocumentsService) CreateJob(ctx context.Context, params DocumentsJobsParams, opts ...RequestOption) (*DocumentJob, error) {
	return post[DocumentJob](ctx, s.c, "POST /v1/documents/jobs", params, opts)
}

// JobInfo reads a report job's progress (POST /v1/documents/jobs/info).
func (s *DocumentsService) JobInfo(ctx context.Context, jobID string, opts ...RequestOption) (*DocumentJob, error) {
	return post[DocumentJob](ctx, s.c, "POST /v1/documents/jobs/info", DocumentsJobsInfoParams{JobID: jobID}, opts)
}

// JobFile downloads a finished job (GET /v1/documents/jobs/file).
func (s *DocumentsService) JobFile(ctx context.Context, jobID string, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "GET /v1/documents/jobs/file", query("job_id", jobID), nil, nil, opts)
}

// Statement renders the account statement for a period (GET /v1/documents/statement).
func (s *DocumentsService) Statement(ctx context.Context, q PeriodQuery, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "GET /v1/documents/statement", periodQuery(q, ""), nil, nil, opts)
}

// BalanceCertificate renders the balance certificate (GET /v1/documents/balance).
func (s *DocumentsService) BalanceCertificate(ctx context.Context, q DocumentQuery, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "GET /v1/documents/balance", query("lang", q.Lang), nil, nil, opts)
}

// FeeSchedule renders the fee schedule in force for the merchant (GET /v1/documents/fees).
func (s *DocumentsService) FeeSchedule(ctx context.Context, q DocumentQuery, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "GET /v1/documents/fees", query("lang", q.Lang), nil, nil, opts)
}

// Ledger exports the full ledger for a period (GET /v1/documents/ledger).
func (s *DocumentsService) Ledger(ctx context.Context, q PeriodQuery, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "GET /v1/documents/ledger", periodQuery(q, ""), nil, nil, opts)
}

// SplitReport shows how one payment was split between partners (GET /v1/documents/split).
func (s *DocumentsService) SplitReport(ctx context.Context, paymentUUID string, q DocumentQuery, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "GET /v1/documents/split", query("lang", q.Lang, "uuid", paymentUUID), nil, nil, opts)
}

// BatchReport is the per-row report of an asynchronous batch (GET /v1/documents/batch). The batch
// id travels as the uuid parameter, which is what the core names it.
func (s *DocumentsService) BatchReport(ctx context.Context, batchID string, q FormatQuery, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "GET /v1/documents/batch",
		query("lang", q.Lang, "format", q.Format, "uuid", batchID), nil, nil, opts)
}

// LinkReport is a payment link's report, covering the invoices it spawned
// (GET /v1/documents/link).
func (s *DocumentsService) LinkReport(ctx context.Context, linkID string, q FormatQuery, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "GET /v1/documents/link",
		query("lang", q.Lang, "format", q.Format, "uuid", linkID), nil, nil, opts)
}

// WalletStatement is a static wallet's statement (GET /v1/documents/wallet/statement).
func (s *DocumentsService) WalletStatement(ctx context.Context, walletUUID string, q PeriodQuery, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "GET /v1/documents/wallet/statement", periodQuery(q, walletUUID), nil, nil, opts)
}

// ReferralsReport is the referral earnings report (GET /v1/documents/referrals).
func (s *DocumentsService) ReferralsReport(ctx context.Context, q PeriodQuery, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "GET /v1/documents/referrals", periodQuery(q, ""), nil, nil, opts)
}

// Download fetches a public document by its signed link (GET /v1/documents/{kind}/{id}): exp and
// sig come from a document_url the API returned. No credentials needed — fetching document_url
// directly does the same thing.
func (s *DocumentsService) Download(ctx context.Context, kind, id string, q DownloadQuery, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "GET /v1/documents/{kind}/{id}",
		query("lang", q.Lang, "exp", strconv.FormatInt(q.Exp, 10), "sig", q.Sig),
		map[string]string{"kind": kind, "id": id}, nil, opts)
}

func periodQuery(q PeriodQuery, uuid string) url.Values {
	return query("lang", q.Lang, "format", q.Format, "from", q.From, "to", q.To, "uuid", uuid)
}
