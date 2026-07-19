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
	// Reference — ваш ключ дедупликации ссылки (уникальный индекс per-merchant, только когда
	// задан). ВТОРОЙ, durable слой защиты от дублей поверх заголовка Idempotency-Key: работает
	// даже без заголовка и даже когда ответ батча слишком велик для кэша идемпотентности
	// (>256 КБ). Дубль даёт payoutlink.duplicate_reference (409), а не реплей первого ответа.
	// Задавайте его всегда — особенно на CreateBatch. См. комментарий к PayoutLinksResource.
	Reference string `json:"reference,omitempty"`
	// IdempotencyKey — свой ключ для заголовка Idempotency-Key (пусто — SDK сгенерирует UUID v4).
	// В теле запроса НЕ передаётся (json:"-"). Заголовок отправляется всегда, и шлюз его УВАЖАЕТ:
	// повтор с тем же ключом реплеит первый ответ. См. комментарий к PayoutLinksResource.
	IdempotencyKey string `json:"-"`
	Title          string `json:"title,omitempty"` // лейбл, виден получателю
	Note           string `json:"note,omitempty"`  // заметка, видна получателю (и в письме)
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
// Management-методы (Create/CreateBatch/List/Info/Cancel) требуют PAYOUT-ключ.
// ClaimInfo/Claim — публичные, без подписи.
//
// # Дедупликация создающих вызовов — ВАЖНО
//
// Create/CreateBatch РЕЗЕРВИРУЮТ деньги, а SDK автоматически повторяет сетевые ошибки и 5xx
// (до 4 попыток). Потерянный ответ поэтому мог бы обернуться несколькими профинансированными
// ссылками. Защита — два слоя:
//
//  1. Заголовок Idempotency-Key. SDK шлёт его на оба эндпоинта, фиксируя ключ ДО цикла повторов,
//     и шлюз его УВАЖАЕТ: /v1/payout/link и /v1/payout/link/batch обёрнуты в идемпотентность.
//     Повтор с тем же ключом реплеит первый ответ (та же ссылка, тот же ClaimToken) с заголовком
//     Idempotent-Replayed: true, а баланс дебетуется РОВНО ОДИН РАЗ. Без заголовка поведение
//     прежнее: два одинаковых вызова создадут ДВЕ ссылки.
//  2. Per-link Reference — durable слой: на (merchant_id, reference) стоит уникальный индекс, и
//     повтор с тем же Reference не создаст вторую ссылку. Работает даже без заголовка.
//
// Коды ответа, специфичные для слоя идемпотентности:
//   - 400 idempotency.key_reused    — тот же ключ с ДРУГИМ телом (терминальная, SDK не ретраит);
//   - 400 idempotency.bad_key       — кривой/слишком длинный ключ (>255) (терминальная);
//   - 409 idempotency.in_progress   — параллельный повтор, пока первый ещё выполняется
//     (терминальная для авто-ретрая; повторите сами чуть позже с ТЕМ ЖЕ ключом);
//   - 503 idempotency.unavailable   — стор идемпотентности недоступен, fail-closed by design
//     (ретраибельная, SDK повторит сам);
//   - 409 payoutlink.duplicate_reference — дубль Reference (терминальная; раньше было 500,
//     которое SDK ретраил впустую).
//
// Оговорки по батчам:
//   - Частично упавший батч реплеится КАК ЕСТЬ: упавшие элементы НЕ повторяются под тем же
//     ключом — шлите их НОВЫМ ключом.
//   - Ответ батча больше 256 КБ шлюз НЕ кэширует, и тогда повтор выполнится заново. Именно
//     поэтому на батчах стоит проставлять per-item Reference — второй слой защиты.
type PayoutLinksResource struct{ c *Client }

// Create создаёт payout-ссылку: резервирует Amount с доступного баланса и возвращает одноразовые
// ClaimToken/ClaimURL. POST /v1/payout/link
//
// РЕКОМЕНДАЦИЯ: задавайте ExpiresInHours явно — при 0/отсутствии бэкенд клампит срок к минимуму,
// и ссылка проживёт всего 1 час (диапазон [1, 720]).
//
// Запрос идёт с заголовком Idempotency-Key (стабилен между внутренними повторами; свой ключ —
// params.IdempotencyKey). Шлюз его уважает: повтор реплеит первый ответ, баланс дебетуется один
// раз. Reference — второй, durable слой защиты. См. комментарий к PayoutLinksResource.
func (r *PayoutLinksResource) Create(ctx context.Context, params PayoutLinkParams) (*PayoutLink, error) {
	var out PayoutLink
	return &out, r.c.requestIdemKey(ctx, "/v1/payout/link", params, params.IdempotencyKey, &out)
}

// CreateBatch создаёт до 500 payout-ссылок одним запросом. POST /v1/payout/link/batch
//
// Каждый элемент резервируется в собственной транзакции: плохой элемент фейлит только себя
// (Results index-aligned с запросом); все созданные ссылки получают общий BatchID. Больше 500
// элементов — batch.too_large (разбейте на страницы). Про ExpiresInHours — см. Create.
//
// Запрос идёт с заголовком Idempotency-Key (стабилен между внутренними повторами); свой ключ на
// весь батч — CreateBatchWithKey. Шлюз его уважает. ДВЕ оговорки именно для батчей: частично
// упавший батч реплеится как есть (упавшие элементы шлите НОВЫМ ключом), а ответ больше 256 КБ
// не кэшируется — поэтому проставляйте per-item Reference. См. комментарий к PayoutLinksResource.
func (r *PayoutLinksResource) CreateBatch(ctx context.Context, links []PayoutLinkParams) (*PayoutLinkBatch, error) {
	return r.CreateBatchWithKey(ctx, links, "")
}

// CreateBatchWithKey — как CreateBatch, но с вашим ключом идемпотентности на весь вызов
// (пусто — SDK сгенерирует UUID v4). Ключ уходит заголовком Idempotency-Key и не входит в подпись.
func (r *PayoutLinksResource) CreateBatchWithKey(ctx context.Context, links []PayoutLinkParams, idempotencyKey string) (*PayoutLinkBatch, error) {
	var out PayoutLinkBatch
	return &out, r.c.requestIdemKey(ctx, "/v1/payout/link/batch", Params{"links": links}, idempotencyKey, &out)
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
