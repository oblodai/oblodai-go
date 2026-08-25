package oblodai

// Status vocabularies, with the predicates worth having: which states are final, and which of the
// final ones mean the merchant actually has the money.

// FinalPaymentStatuses are the invoice states after which nothing else can happen.
var FinalPaymentStatuses = []PaymentStatus{
	PaymentStatusPaid, PaymentStatusPaidOver, PaymentStatusWrongAmount,
	PaymentStatusExpired, PaymentStatusCancelled,
}

// FinalPayoutStatuses are the payout states after which nothing else can happen.
var FinalPayoutStatuses = []PayoutStatus{
	PayoutStatusConfirmed, PayoutStatusFailed, PayoutStatusCancelled,
}

// IsPaymentFinal reports whether an invoice reached a state nothing follows.
func IsPaymentFinal(status PaymentStatus) bool {
	for _, s := range FinalPaymentStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// IsPaymentPaid reports whether the merchant has the money: paid or paid_over. wrong_amount is
// NOT paid — it is an underpayment waiting for Refunds.Resolve.
func IsPaymentPaid(status PaymentStatus) bool {
	return status == PaymentStatusPaid || status == PaymentStatusPaidOver
}

// IsPaymentUnderpaid reports an invoice waiting for a merchant decision: accept the short payment
// or send it back with Refunds.Resolve.
func IsPaymentUnderpaid(status PaymentStatus) bool {
	return status == PaymentStatusWrongAmount
}

// IsPayoutFinal reports whether a payout reached a state nothing follows.
func IsPayoutFinal(status PayoutStatus) bool {
	for _, s := range FinalPayoutStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// IsPayoutSucceeded reports whether a payout reached the chain and is irreversible.
func IsPayoutSucceeded(status PayoutStatus) bool { return status == PayoutStatusConfirmed }
