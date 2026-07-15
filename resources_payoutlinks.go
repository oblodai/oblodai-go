package oblodai

import (
	"context"
	"net/url"
)

// ─────────────────────────────── Payout links (крипто-чеки) ───────────────────────────────

// Статусы payout-ссылки (PayoutLink.Status).
const (
	PayoutLinkStatusFunded    = "funded"    // создана, резерв удержан, ждёт claim
	PayoutLinkStatusClaiming  = "claiming"  // claim в процессе: адрес зафиксирован, выплата порождается
	PayoutLinkStatusClaimed   = "claimed"   // выплата порождена (payout_id установлен) — терминальный
	PayoutLinkStatusExpired   = "expired"   // срок истёк без claim, резерв возвращён — терминальный
	PayoutLinkStatusCancelled = "cancelled" // отменена мерчантом до claim, резерв возвращён — терминальный
)

// PayoutLinkParams — параметры создания payout-ссылки (PayoutLinks.Create / CreateBatch).
type PayoutLinkParams struct {
	Currency string `json:"currency"` // только крипто-актив
	Network  string `json:"network"`  // сеть выплаты получателю
	Amount   string `json:"amount"`   // human-строка в currency, > 0
	// Reference — ваш ключ дедупликации ссылки (уникален per-merchant). Именно он защищает от
	// дублей: заголовок Idempotency-Key на payout-link-эндпоинты не шлётся.
	Reference string `json:"reference,omitempty"`
	Title     string `json:"title,omitempty"` // лейбл, виден получателю
	Note      string `json:"note,omitempty"`  // заметка, видна получателю (и в письме)
	// Email — при заполнении получателю уходит письмо со ссылкой claim (best-effort).
	Email string `json:"email,omitempty"`
	// ExpiresInHours — окно claim в часах, клампится бэкендом в [1, 720]. ЗАДАВАЙТЕ ЯВНО:
	// при 0/отсутствии бэкенд клампит к МИНИМУМУ — ссылка проживёт всего 1 час.
	ExpiresInHours int `json:"expires_in_hours,omitempty"`
}

// PayoutLink — payout-ссылка (крипто-чек). ClaimToken/ClaimURL заполнены ТОЛЬКО в ответе
// Create/CreateBatch — сохраните их сразу, повторно получить нельзя (хранится только хеш).
type PayoutLink struct {
	LinkID       string `json:"link_id"`
	Status       string `json:"status"` // см. PayoutLinkStatus*
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	Network      string `json:"network"`
	Title        string `json:"title,omitempty"`
	Note         string `json:"note,omitempty"`
	ExpiresAt    string `json:"expires_at"`
	CreatedAt    string `json:"created_at"`
	Reference    string `json:"reference,omitempty"`
	Email        string `json:"email,omitempty"`
	PayoutID     string `json:"payout_id,omitempty"`     // после claim
	ClaimAddress string `json:"claim_address,omitempty"` // после claim
	BatchID      string `json:"batch_id,omitempty"`      // общий id вызова CreateBatch
	ClaimToken   string `json:"claim_token,omitempty"`   // только в ответе create
	ClaimURL     string `json:"claim_url,omitempty"`     // только в ответе create
}

// PayoutLinkBatchItem — результат одного элемента CreateBatch (index-aligned с запросом).
type PayoutLinkBatchItem struct {
	OK      bool        `json:"ok"`
	Link    *PayoutLink `json:"link,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

// PayoutLinkBatch — результат PayoutLinks.CreateBatch.
type PayoutLinkBatch struct {
	Created int                   `json:"created"`
	Total   int                   `json:"total"`
	Results []PayoutLinkBatchItem `json:"results"`
}

// PayoutLinkClaimInfo — публичные детали ссылки для страницы claim (ничего мерчант-приватного).
type PayoutLinkClaimInfo struct {
	Status    string `json:"status"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Network   string `json:"network"`
	Title     string `json:"title,omitempty"`
	Note      string `json:"note,omitempty"`
	ExpiresAt string `json:"expires_at"`
	Claimable bool   `json:"claimable"`
}

// PayoutLinkClaim — результат успешного claim.
type PayoutLinkClaim struct {
	Status   string `json:"status"` // "claimed"
	PayoutID string `json:"payout_id"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Network  string `json:"network"`
	Address  string `json:"address"`
}

// PayoutLinksResource — claimable payout-ссылки («крипто-чеки»): мерчант резервирует средства, НЕ
// зная кошелька получателя; получатель открывает публичную страницу claim и вводит свой адрес.
//
// Management-методы (Create/CreateBatch/List/Info/Cancel) требуют PAYOUT-ключ. Заголовок
// Idempotency-Key на них НЕ шлётся — эти эндпоинты не обёрнуты в идемпотентность на шлюзе;
// защита от дублей — ваш per-link Reference (уникальный индекс per-merchant). ClaimInfo/Claim —
// публичные, без подписи.
type PayoutLinksResource struct{ c *Client }

// Create создаёт payout-ссылку: резервирует Amount с доступного баланса и возвращает одноразовые
// ClaimToken/ClaimURL. POST /v1/payout/link
//
// РЕКОМЕНДАЦИЯ: задавайте ExpiresInHours явно — при 0/отсутствии бэкенд клампит срок к минимуму,
// и ссылка проживёт всего 1 час (диапазон [1, 720]). Для защиты от дублей задавайте Reference:
// заголовок Idempotency-Key на этот эндпоинт не шлётся.
func (r *PayoutLinksResource) Create(ctx context.Context, params PayoutLinkParams) (*PayoutLink, error) {
	var out PayoutLink
	return &out, r.c.request(ctx, "/v1/payout/link", params, &out)
}

// CreateBatch создаёт до 500 payout-ссылок одним запросом. POST /v1/payout/link/batch
//
// Каждый элемент резервируется в собственной транзакции: плохой элемент фейлит только себя
// (Results index-aligned с запросом); все созданные ссылки получают общий BatchID. Больше 500
// элементов — batch.too_large (разбейте на страницы). Заголовок Idempotency-Key не шлётся —
// задавайте per-link Reference. Про ExpiresInHours — см. Create.
func (r *PayoutLinksResource) CreateBatch(ctx context.Context, links []PayoutLinkParams) (*PayoutLinkBatch, error) {
	var out PayoutLinkBatch
	return &out, r.c.request(ctx, "/v1/payout/link/batch", Params{"links": links}, &out)
}

// List возвращает страницу payout-ссылок (created_at DESC, без claim-токенов).
// POST /v1/payout/link/list — limit ≤0 или >200 бэкенд заменяет на 50.
func (r *PayoutLinksResource) List(ctx context.Context, limit, offset int) ([]PayoutLink, error) {
	var out struct {
		Links []PayoutLink `json:"links"`
	}
	if err := r.c.request(ctx, "/v1/payout/link/list", Params{"limit": limit, "offset": offset}, &out); err != nil {
		return nil, err
	}
	return out.Links, nil
}

// Info возвращает payout-ссылку по id (после claim — с PayoutID и ClaimAddress; claim-токен не
// возвращается никогда). POST /v1/payout/link/info
func (r *PayoutLinksResource) Info(ctx context.Context, linkID string) (*PayoutLink, error) {
	var out PayoutLink
	return &out, r.c.request(ctx, "/v1/payout/link/info", Params{"link_id": linkID}, &out)
}

// Cancel отменяет НЕполученную (funded) ссылку и возвращает резерв на доступный баланс.
// POST /v1/payout/link/cancel — иной статус даёт payoutlink.not_funded (409).
func (r *PayoutLinksResource) Cancel(ctx context.Context, linkID string) (*PayoutLink, error) {
	var out PayoutLink
	return &out, r.c.request(ctx, "/v1/payout/link/cancel", Params{"link_id": linkID}, &out)
}

// ClaimInfo возвращает публичные детали ссылки по claim-токену (для своей страницы claim).
// GET /v1/claim/{token} — ПУБЛИЧНЫЙ, запрос уходит БЕЗ подписи (ключи не нужны).
func (r *PayoutLinksResource) ClaimInfo(ctx context.Context, token string) (*PayoutLinkClaimInfo, error) {
	var out PayoutLinkClaimInfo
	return &out, r.c.requestPublicGET(ctx, "/v1/claim/"+url.PathEscape(token), &out)
}

// Claim получает средства ссылки на адрес получателя: из резерва порождается обычная выплата.
// POST /v1/claim/{token} — ПУБЛИЧНЫЙ, запрос уходит БЕЗ подписи (ключи не нужны).
//
// Идемпотентен: повторный claim уже полученной ссылки возвращает ту же выплату; claim с ДРУГИМ
// адресом после первого — payoutlink.claim_in_progress (409). Для сетей с memo/dest tag (TON и
// т.п.) используйте ClaimWithMemo.
func (r *PayoutLinksResource) Claim(ctx context.Context, token, address string) (*PayoutLinkClaim, error) {
	return r.ClaimWithMemo(ctx, token, address, "")
}

// ClaimWithMemo — как Claim, но с memo/dest tag/comment для сетей, где он нужен (TON и т.п.).
// POST /v1/claim/{token} — ПУБЛИЧНЫЙ, без подписи.
func (r *PayoutLinksResource) ClaimWithMemo(ctx context.Context, token, address, memo string) (*PayoutLinkClaim, error) {
	body := Params{"address": address}
	if memo != "" {
		body["memo"] = memo
	}
	var out PayoutLinkClaim
	return &out, r.c.requestPublic(ctx, "/v1/claim/"+url.PathEscape(token), body, &out)
}
