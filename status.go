package oblodai

// Status vocabularies, with the predicates worth having: which states are final, and which of the
// final ones mean the merchant actually has the money. Which values are final and which succeed is
// a fact of the contract (x-status-classes): the generator writes it next to each enumeration
// (PaymentStatusFinalValues, PaymentStatus.IsFinal, …); the helpers here are the 2.0 names for it.

// FinalPaymentStatuses are the invoice states after which nothing else can happen.
var FinalPaymentStatuses = PaymentStatusFinalValues

// FinalPayoutStatuses are the payout states after which nothing else can happen.
var FinalPayoutStatuses = PayoutStatusFinalValues

// IsPaymentFinal reports whether an invoice reached a state nothing follows.
func IsPaymentFinal(status PaymentStatus) bool { return status.IsFinal() }

// IsPaymentPaid reports whether the merchant has the money: paid or paid_over. wrong_amount is
// NOT paid — it is an underpayment waiting for Refunds.Resolve.
func IsPaymentPaid(status PaymentStatus) bool { return status.IsSuccess() }

// IsPaymentUnderpaid reports an invoice waiting for a merchant decision: accept the short payment
// or send it back with Refunds.Resolve.
func IsPaymentUnderpaid(status PaymentStatus) bool { return status == PaymentStatusWrongAmount }

// IsPayoutFinal reports whether a payout reached a state nothing follows.
func IsPayoutFinal(status PayoutStatus) bool { return status.IsFinal() }

// IsPayoutSucceeded reports whether a payout reached the chain and is irreversible.
func IsPayoutSucceeded(status PayoutStatus) bool { return status.IsSuccess() }
