package oblodai

import "context"

// ─────────────────────────────── Wallets ───────────────────────────────

// WalletsResource — методы статических кошельков.
type WalletsResource struct{ c *Client }

// Create создаёт (или получает) постоянный статический адрес. POST /v1/wallet
func (r *WalletsResource) Create(ctx context.Context, params Params) (*Wallet, error) {
	var out Wallet
	return &out, r.c.request(ctx, "/v1/wallet", params, &out)
}

// Block блокирует/разблокирует кошелёк. POST /v1/wallet/block
// forceBlock: nil — заблокировать (дефолт API); &false — разблокировать; &true — заблокировать явно.
func (r *WalletsResource) Block(ctx context.Context, address string, forceBlock *bool) (map[string]any, error) {
	body := Params{"address": address}
	if forceBlock != nil {
		body["is_force_block"] = *forceBlock
	}
	var out map[string]any
	return out, r.c.request(ctx, "/v1/wallet/block", body, &out)
}

// BlockedAddressRefund возвращает средства с кошелька на адрес. POST /v1/wallet/blocked-address-refund
func (r *WalletsResource) BlockedAddressRefund(ctx context.Context, uuid, address string) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/wallet/blocked-address-refund", Params{"uuid": uuid, "address": address}, &out)
}

// QR возвращает QR-код произвольного адреса. POST /v1/wallet/qr
func (r *WalletsResource) QR(ctx context.Context, address string) (map[string]string, error) {
	var out map[string]string
	return out, r.c.request(ctx, "/v1/wallet/qr", Params{"address": address}, &out)
}

// ─────────────────────────────── Account ───────────────────────────────

// AccountResource — баланс, рефералы, перевод на личный кошелёк, VRCS.
type AccountResource struct{ c *Client }

// Balance возвращает доступные балансы мерчанта. POST /v1/balance
func (r *AccountResource) Balance(ctx context.Context) (*Balance, error) {
	var wrapper struct {
		Balance Balance `json:"balance"`
	}
	if err := r.c.request(ctx, "/v1/balance", Params{}, &wrapper); err != nil {
		return nil, err
	}
	return &wrapper.Balance, nil
}

// Referral возвращает реферальную статистику. POST /v1/referral/info
func (r *AccountResource) Referral(ctx context.Context) (*ReferralInfo, error) {
	var out ReferralInfo
	return &out, r.c.request(ctx, "/v1/referral/info", Params{}, &out)
}

// TransferToPersonal переводит средства на личный кошелёк владельца. POST /v1/transfer/to-personal
//
// Защита от дублей при повторах — заголовок Idempotency-Key (генерируется один раз до цикла
// повторов; свой ключ — в params["idempotency_key"]). order_id уходит как есть — SDK его больше
// не подставляет (ломающее изменение v1.1.0).
func (r *AccountResource) TransferToPersonal(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.requestIdem(ctx, "/v1/transfer/to-personal", params, &out)
}

// TransferToUserResult — результат Account.TransferToUser (внутреннего перевода пользователю).
type TransferToUserResult struct {
	Currency         string `json:"currency"`
	Amount           string `json:"amount"`
	ToUserID         string `json:"to_user_id"`
	RecipientBalance string `json:"recipient_balance"`
}

// TransferToUser переводит средства с баланса мерчанта на личный кошелёк ДРУГОГО пользователя
// платформы — внутренний перевод без комиссии. POST /v1/transfer/to-user
//
// params: "to_user_id" — id пользователя-получателя (UUID платформы, НЕ username: username
// резолвится в id на стороне кабинета), "amount" и "currency" — строки, "order_id" — необязателен.
// Требует PAYOUT-ключ. Защита от дублей — та же лестница, что у остальных денежных эндпоинтов
// (заголовок Idempotency-Key → order_id → подпись): SDK шлёт заголовок Idempotency-Key,
// сгенерированный один раз до цикла повторов; свой ключ — в params["idempotency_key"].
func (r *AccountResource) TransferToUser(ctx context.Context, params Params) (*TransferToUserResult, error) {
	var out TransferToUserResult
	return &out, r.c.requestIdem(ctx, "/v1/transfer/to-user", params, &out)
}

// TransferBatch ставит в обработку пачку внутренних переводов пользователям (до 5000) одним
// подписанным запросом — «зарплатный» сценарий. POST /v1/transfer/batch
//
// Каждый элемент — тело обычного Account.TransferToUser. onError: "continue" (по умолчанию) или
// "stop". Обработка фоновая: прогресс и результаты по каждому элементу — через
// Batches.Info(batchID, ...). Требует PAYOUT-ключ. Запрос идёт с заголовком Idempotency-Key
// (стабилен между повторами).
func (r *AccountResource) TransferBatch(ctx context.Context, transfers []Params, onError string) (*BatchSubmission, error) {
	body := Params{"transfers": transfers}
	if onError != "" {
		body["on_error"] = onError
	}
	var out BatchSubmission
	return &out, r.c.requestIdem(ctx, "/v1/transfer/batch", body, &out)
}

// VRCS включает/выключает VRCS. enabled nil — чтение. POST /v1/vrcs
func (r *AccountResource) VRCS(ctx context.Context, enabled *bool) (map[string]any, error) {
	body := Params{}
	if enabled != nil {
		body["enabled"] = *enabled
	}
	var out map[string]any
	return out, r.c.request(ctx, "/v1/vrcs", body, &out)
}

// ─────────────────────────────── Webhooks ───────────────────────────────

// WebhooksResource — управление вебхуками и тестовые события.
// Проверка ВХОДЯЩИХ вебхуков — функции VerifyWebhook / ConstructEvent.
type WebhooksResource struct{ c *Client }

// Register регистрирует URL для вебхуков и возвращает секрет. POST /v1/webhooks
func (r *WebhooksResource) Register(ctx context.Context, url string) (*WebhookRegistration, error) {
	var out WebhookRegistration
	return &out, r.c.request(ctx, "/v1/webhooks", Params{"url": url}, &out)
}

// Deliveries возвращает журнал последних доставок. POST /v1/webhooks/deliveries
func (r *WebhooksResource) Deliveries(ctx context.Context) ([]Delivery, error) {
	var wrapper struct {
		Deliveries []Delivery `json:"deliveries"`
	}
	if err := r.c.request(ctx, "/v1/webhooks/deliveries", Params{}, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Deliveries, nil
}

// TestPayment отправляет пробный вебхук платежа. POST /v1/test-webhook/payment
func (r *WebhooksResource) TestPayment(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/test-webhook/payment", params, &out)
}

// TestWallet отправляет пробный вебхук кошелька. POST /v1/test-webhook/wallet
func (r *WebhooksResource) TestWallet(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/test-webhook/wallet", params, &out)
}

// TestPayout отправляет пробный вебхук выплаты. POST /v1/test-webhook/payout
func (r *WebhooksResource) TestPayout(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/test-webhook/payout", params, &out)
}

// ─────────────────────────────── Settings ───────────────────────────────

// SettingsResource — автовывод и IP-allowlist.
type SettingsResource struct{ c *Client }

// ListAutoWithdraw возвращает правила автовывода. POST /v1/auto-withdraw/list
func (r *SettingsResource) ListAutoWithdraw(ctx context.Context) ([]AutoWithdrawRule, error) {
	var wrapper struct {
		Rules []AutoWithdrawRule `json:"rules"`
	}
	if err := r.c.request(ctx, "/v1/auto-withdraw/list", Params{}, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Rules, nil
}

// SetAutoWithdraw включает автовывод для актива. POST /v1/auto-withdraw/set
func (r *SettingsResource) SetAutoWithdraw(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/auto-withdraw/set", params, &out)
}

// DeleteAutoWithdraw выключает автовывод для актива. POST /v1/auto-withdraw/delete
func (r *SettingsResource) DeleteAutoWithdraw(ctx context.Context, currency string) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/auto-withdraw/delete", Params{"currency": currency}, &out)
}

// ListAllowlist возвращает список доверенных IP и статус. POST /v1/api-allowlist/list
func (r *SettingsResource) ListAllowlist(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/api-allowlist/list", Params{}, &out)
}

// AddAllowlist добавляет IP или CIDR. POST /v1/api-allowlist/add
func (r *SettingsResource) AddAllowlist(ctx context.Context, cidr string) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/api-allowlist/add", Params{"cidr": cidr}, &out)
}

// RemoveAllowlist удаляет IP или CIDR. POST /v1/api-allowlist/remove
func (r *SettingsResource) RemoveAllowlist(ctx context.Context, cidr string) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/api-allowlist/remove", Params{"cidr": cidr}, &out)
}

// EnableAllowlist включает/выключает контроль. POST /v1/api-allowlist/enable
func (r *SettingsResource) EnableAllowlist(ctx context.Context, enabled bool) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/api-allowlist/enable", Params{"enabled": enabled}, &out)
}

// ─────────────────────────────── Rates ───────────────────────────────

// RatesResource — публичные курсы валют (подпись не требуется).
type RatesResource struct{ c *Client }

// List возвращает курсы к USDT. currencyFrom пустой — по всем валютам. POST /v1/exchange-rate/list
func (r *RatesResource) List(ctx context.Context, currencyFrom string) ([]ExchangeRate, error) {
	body := Params{}
	if currencyFrom != "" {
		body["currency_from"] = currencyFrom
	}
	var out []ExchangeRate
	return out, r.c.requestPublic(ctx, "/v1/exchange-rate/list", body, &out)
}

// Currencies возвращает публичный каталог активов и сетей. GET /v1/currencies (без подписи).
func (r *RatesResource) Currencies(ctx context.Context) ([]Currency, error) {
	var out struct {
		Currencies []Currency `json:"currencies"`
	}
	if err := r.c.requestPublicGET(ctx, "/v1/currencies", &out); err != nil {
		return nil, err
	}
	return out.Currencies, nil
}
