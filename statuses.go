package oblodai

// Словари статусов шлюза. Значения — ровно те строки, что приходят в JSON, поэтому сравнение с
// литералом (`p.PaymentStatus == "paid"`) продолжает работать; именованные типы нужны, чтобы
// опечатка в статусе ловилась компилятором.

// PaymentStatus — статус платежа (инвойса), поле `payment_status`.
type PaymentStatus string

// Статусы платежа. Соответствуют apifmt.PaymentStatus / apifmt.PaymentStatusResolved в ядре.
const (
	// PaymentStatusCheck — счёт создан, оплаты ещё не видели.
	PaymentStatusCheck PaymentStatus = "check"
	// PaymentStatusConfirmCheck — оплата увидена, ждём подтверждений сети.
	PaymentStatusConfirmCheck PaymentStatus = "confirm_check"
	// PaymentStatusWrongAmountWaiting — увидели ЧАСТИЧНУЮ оплату и ждём доплату. НЕ терминальный:
	// счёт ещё может стать paid поздней доплатой. Payments.Resolve здесь отвечает 409
	// resolution.not_underpaid — решать судьбу недоплаты можно только после закрытия счёта
	// (PaymentStatusWrongAmount).
	PaymentStatusWrongAmountWaiting PaymentStatus = "wrong_amount_waiting"
	// PaymentStatusWrongAmount — счёт закрылся недоплаченным. Вот теперь доступен Payments.Resolve
	// (accept — оставить частичную оплату себе, refund — вернуть плательщику).
	PaymentStatusWrongAmount PaymentStatus = "wrong_amount"
	// PaymentStatusPaid — оплачен полностью. Терминальный.
	PaymentStatusPaid PaymentStatus = "paid"
	// PaymentStatusPaidOver — переплачен. Терминальный. Излишек уходит в авто-возврат, если он
	// включён (Payments.SetAutorefund) и сеть его поддерживает (UTXO-сети — нет).
	PaymentStatusPaidOver PaymentStatus = "paid_over"
	// PaymentStatusCancel — истёк или отменён. Терминальный.
	PaymentStatusCancel PaymentStatus = "cancel"
	// PaymentStatusSelect — валюто-агностичный счёт: покупатель ещё не выбрал валюту и сеть.
	// Адреса и суммы к оплате ещё нет — их выдаёт Payments.PublicSelect.
	PaymentStatusSelect PaymentStatus = "select"
)

// IsFinal сообщает, что по счёту больше не ожидается автоматических переходов: paid, paid_over,
// cancel и закрывшийся wrong_amount. Совпадает с полем `is_final` в ответе шлюза — им и стоит
// пользоваться, когда оно есть; этот метод удобен, когда на руках только строка статуса.
//
// Внимание: wrong_amount_waiting НЕ терминален (ждём доплату), а wrong_amount — терминален.
func (s PaymentStatus) IsFinal() bool {
	switch s {
	case PaymentStatusPaid, PaymentStatusPaidOver, PaymentStatusWrongAmount, PaymentStatusCancel:
		return true
	default:
		return false
	}
}

// PayoutStatus — статус выплаты, поле `status` у Payout.
type PayoutStatus string

// Статусы выплаты. Соответствуют apifmt.PayoutStatus в ядре.
const (
	// PayoutStatusCheck — создана, ждёт одобрения (см. Payouts.Approve и поле ApprovalRequired).
	PayoutStatusCheck PayoutStatus = "check"
	// PayoutStatusProcess — одобрена и уходит/ушла в сеть (внутри — approved/broadcasting/sent).
	PayoutStatusProcess PayoutStatus = "process"
	// PayoutStatusPaid — подтверждена сетью. Терминальный.
	PayoutStatusPaid PayoutStatus = "paid"
	// PayoutStatusFail — не удалась. Терминальный.
	PayoutStatusFail PayoutStatus = "fail"
	// PayoutStatusCancel — отменена. Терминальный.
	PayoutStatusCancel PayoutStatus = "cancel"
)

// IsFinal сообщает, что выплата дошла до конца: paid, fail или cancel. Совпадает с полем
// `is_final` в ответе шлюза.
func (s PayoutStatus) IsFinal() bool {
	switch s {
	case PayoutStatusPaid, PayoutStatusFail, PayoutStatusCancel:
		return true
	default:
		return false
	}
}
