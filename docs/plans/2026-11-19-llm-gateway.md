# LLM Gateway（作品集）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use the execution skill chosen at handoff (subagent-driven-development or executing-plans) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 精簡版 LLM API 閘道（類 NewAPI）：OpenAI 格式統一入口、雙通路自動切換、預付扣款、Key 限流、TOTP 登入、開發者控制台與管理後台，Docker Compose 一鍵起。

**Architecture:** 單一 Go（Gin）後端同時提供 `/v1/*` 轉發 API（API Key 認證）與 `/api/*` 控制台 API（JWT httpOnly cookie 認證）；MySQL 存資料、Redis 做限流；另一個 Go 小程式 `mockupstream` 同時扮演 OpenAI 與 Anthropic 上游（可切換成故障模式示範 failover）。React + Vite 前端 build 後由 nginx 提供並反代 `/api`、`/v1`。

**Tech Stack:** Go 1.27、Gin、GORM + MySQL 8、go-redis v9、pquerna/otp、golang-jwt v5、bcrypt；React 18 + Vite + TypeScript + react-router；Docker Compose；GitHub Actions；Playwright；gosec、Semgrep。

**Spec:** this plan — Design section（Steven 2026-11-19 的需求訊息）

## Design

**需求對照（全部要做到）**
1. `POST /v1/chat/completions` OpenAI 格式；後面 OpenAI、Anthropic 兩種通路；上游 5xx／逾時／連線失敗自動換下一個通路
2. MySQL：users、api_keys、channels、model_prices、usage_logs、topups；Redis：每把 Key 每分鐘請求數限流
3. 每次呼叫記 prompt／completion tokens 並從預付餘額扣款；餘額不足回 402；有儲值 API
4. 登入：密碼（bcrypt）＋ TOTP；角色 `user`（開發者）／`admin`
5. 前端：開發者控制台（申請／停用 Key、用量、餘額、儲值、兩步驟驗證設定）＋管理後台（使用者、通路、價格）
6. Docker Compose 一鍵起、GitHub Actions 跑測試、`docs/openapi.yaml`、README；上游一律用假模型，不花錢
7. 完成後 gosec + Semgrep 稽核，寫 `docs/security-audit.md`

**關鍵設計決定**
- **金額用整數**：餘額與價格單位為 micro-USD（1 USD = 1,000,000），`int64`，不用浮點。價格存「每 1M tokens 的 micro-USD」，成本 = `ceil((prompt*in + completion*out) / 1_000_000)`
- **扣款採「預扣再結算」**（NewAPI 的做法）：呼叫前依 `估計 prompt tokens（字元數/4）+ max_tokens（未給用 1024）` 算預估成本，以 `UPDATE users SET balance=balance-? WHERE id=? AND balance>=?` 原子預扣，影響 0 列 → 402；呼叫完依實際 usage 退差額或補扣；上游全失敗則全額退回。併發也不會扣成負數
- **Failover 規則**：同模型的可用通路依 `priority` 由小到大試；只有上游 5xx、429、逾時（預設 30s）、連線錯誤才換下一個，上游 4xx（請求本身錯）直接回給使用者不切換
- **跨廠牌同模型**：通路有 `model_map`（JSON），例如 Anthropic 通路把 `gpt-4o-mini` 對應成 `claude-haiku-4-5`，所以 OpenAI 掛了能切 Anthropic；計價以使用者請求的模型名為準
- **API Key**：格式 `sk-` + 48 字元隨機；DB 只存 SHA-256 雜湊與前 8 碼顯示用，建立時明文只回一次
- **登入 session**：JWT（HS256，12 小時）放 httpOnly + SameSite=Lax cookie；開了 TOTP 的帳號密碼對了先拿 5 分鐘的 `pending_totp` token，再送 6 位數碼換正式 cookie
- **限流**：Redis 固定視窗 `rl:{keyID}:{unix分鐘}` INCR + EXPIRE 60s，超過該 Key 的 `rpm_limit`（預設 60）回 429；Redis 掛掉時放行並記 warning（fail-open，報告裡寫明取捨）
- **儲值**：`POST /api/billing/topup {amount_usd}` 模擬付款直接入帳並寫 topups 紀錄（單筆 1–1000 USD）；管理員另有 `POST /api/admin/users/:id/credit` 手動調整
- **不做（YAGNI）**：串流 `stream=true`（回 400 說明不支援）、真實金流、多租戶、Email 驗證
- **錯誤格式**：`/v1/*` 一律 OpenAI 錯誤格式 `{"error":{"message","type","code"}}`

**資料表（GORM AutoMigrate）**
- `users`: id, email(unique), password_hash, role(enum user/admin), totp_secret(nullable), totp_enabled bool, balance int64, disabled bool, created_at
- `api_keys`: id, user_id, name, key_hash(unique), key_prefix, rpm_limit int(預設 60), disabled bool, last_used_at, created_at
- `channels`: id, name, type(openai/anthropic), base_url, api_key, models(JSON 陣列), model_map(JSON), priority int, enabled bool
- `model_prices`: model(PK), input_per_m int64, output_per_m int64
- `usage_logs`: id, user_id, api_key_id, channel_id, model, prompt_tokens, completion_tokens, cost int64, status_code, latency_ms, failover_count, created_at；index(user_id, created_at)
- `topups`: id, user_id, amount int64, source(self/admin), created_at

## Global Constraints

- GitHub 用個人帳號 `Chunweiwu518`，repo `Chunweiwu518/llm-gateway`，**private**
- README、commit 訊息、程式註解都不提 AI／模型輔助，commit 不加 Co-Authored-By
- 測試與 demo 只打 `mockupstream`，任何設定檔／CI 都不放真的 OpenAI／Anthropic key
- secrets（JWT secret、DB 密碼、管理員初始密碼）從環境變數讀，repo 只放 `.env.example`
- Go 裝在 `~/.local/go`（官方 tarball go1.27.4），不動系統套件
- 函式 < 50 行、檔案 < 800 行；不寫 silent except（錯誤至少 log）

## Review Focus

1. **併發扣款**：同一使用者同時打 20 個請求、餘額只夠 5 個 → 只有 5 個成功、其餘 402、餘額不為負（Task 6 測試）
2. **上游 4xx 不該切換**：Anthropic 回 400 時不應再打 OpenAI 也不應扣錢（Task 5 測試）
3. **全部通路都掛**：回 502 OpenAI 錯誤格式、預扣全額退回、usage_logs 記一筆 status 502 cost 0（Task 6 測試）
4. **停用的 Key／停用的使用者**：立刻回 401，不經過限流與扣款（Task 4 測試）
5. **TOTP 未完成的 pending token 不能拿去打 `/api/*`**（Task 3 測試）

---

## File Structure

```
llm-gateway/
├── backend/
│   ├── cmd/gateway/main.go          # 組裝設定、DB、Redis、路由
│   ├── internal/config/config.go    # env → Config
│   ├── internal/store/              # models.go, db.go（連線+AutoMigrate+seed admin）
│   ├── internal/auth/               # password.go, jwt.go, totp.go, handlers.go, middleware.go
│   ├── internal/apikey/             # key.go（產生/雜湊）, handlers.go, middleware.go（/v1 認證）
│   ├── internal/ratelimit/limiter.go
│   ├── internal/relay/              # types.go（OpenAI 格式）, openai.go, anthropic.go, router.go（failover）, handler.go
│   ├── internal/billing/            # pricing.go（成本計算）, ledger.go（預扣/結算/儲值）, handlers.go
│   ├── internal/admin/handlers.go   # 使用者、通路、價格 CRUD
│   └── internal/httpx/errors.go     # OpenAI 錯誤格式 + API 錯誤格式
├── mockupstream/main.go             # 假 OpenAI + 假 Anthropic + /_control 故障開關
├── web/                             # React + Vite + TS
│   ├── src/api.ts                   # fetch 包裝（檢查 res.ok，錯誤丟出顯示）
│   ├── src/pages/                   # Login, Register, Dashboard, Keys, Usage, Billing, Security, admin/{Users,Channels,Prices}
│   └── nginx.conf
├── e2e/                             # Playwright 驗收
├── docs/openapi.yaml, docs/security-audit.md
├── docker-compose.yml, .env.example, .github/workflows/ci.yml, README.md
```

---

### Task 1: 環境與骨架

**Files:** Create `backend/go.mod`, `backend/cmd/gateway/main.go`, `backend/internal/config/config.go`, `backend/internal/store/{models.go,db.go}`, `docker-compose.yml`（先只有 mysql、redis）, `.env.example`, `.gitignore`

**Interfaces — Produces:**
- `config.Load() (Config, error)`：欄位 `DSN, RedisAddr, JWTSecret, AdminEmail, AdminPassword, ListenAddr, UpstreamTimeout time.Duration`
- `store.Open(dsn string) (*gorm.DB, error)`（含 AutoMigrate）；`store.SeedAdmin(db, email, password) error`（不存在才建，role=admin）
- 上面 Design 的六個 model struct
- `GET /healthz` → `{"status":"ok","commit":"<ldflags 注入>"}`

- [ ] Step 1: 裝 Go：下載 `go1.27.4.linux-amd64.tar.gz` 解到 `~/.local/go`，PATH 加 `~/.local/go/bin`；`go version` 應印 `go1.27.4`
- [ ] Step 2: `git init`，設本 repo 的 user.name／user.email 為個人帳號 noreply（`gh api user` 用 Chunweiwu518 查 id）
- [ ] Step 3: 寫 `store` 的測試 `TestSeedAdminIdempotent`：呼叫兩次 SeedAdmin，users 表只有 1 筆 role=admin。測試用環境變數 `TEST_MYSQL_DSN`，沒設就 `t.Skip`（CI 一定會設）
- [ ] Step 4: `docker compose up -d mysql redis` 後跑 `go test ./internal/store/...` → FAIL，實作後 PASS
- [ ] Step 5: commit `chore: project skeleton, config, schema`

### Task 2: 假上游 mockupstream

**Files:** Create `mockupstream/main.go`, `mockupstream/main_test.go`

**Interfaces — Produces:**
- `POST /openai/v1/chat/completions`：回 OpenAI 格式，content = `"[mock-openai] " + 最後一則 user 訊息`，usage：prompt = 所有訊息字元數/4（至少 1），completion = content 字元數/4（至少 1）
- `POST /anthropic/v1/messages`：檢查 `x-api-key`、`anthropic-version` header，回 Anthropic 格式（`content[{type:text}]`、`usage.input_tokens/output_tokens`）
- `POST /_control {"provider":"openai|anthropic","mode":"ok|500|timeout|400"}`：切換該 provider 行為；`GET /_control` 看目前狀態

- [ ] Step 1: 測試 `TestOpenAIOK`、`TestAnthropicOK`、`TestControlMode500`（切 500 後 openai 回 500）、`TestAnthropicMissingVersion400`
- [ ] Step 2: 跑 → FAIL；實作；跑 → PASS
- [ ] Step 3: commit `feat: mock upstream for openai and anthropic`

### Task 3: 註冊登入 TOTP 角色

**Files:** Create `backend/internal/auth/{password.go,jwt.go,totp.go,handlers.go,middleware.go,auth_test.go}`, `backend/internal/httpx/errors.go`

**Interfaces — Produces:**
- `auth.RequireUser() gin.HandlerFunc`（把 `*store.User` 放進 `c.Set("user")`）、`auth.RequireAdmin() gin.HandlerFunc`
- 路由：`POST /api/auth/register {email,password}`（密碼至少 8 碼）、`POST /api/auth/login {email,password}` → 未開 TOTP 直接設 cookie 回 `{user}`；開了回 `{"totp_required":true}` 並設 `pending` cookie、`POST /api/auth/login/totp {code}`、`POST /api/auth/logout`、`GET /api/me`、`POST /api/me/totp/setup` → `{otpauth_url, secret}`、`POST /api/me/totp/enable {code}`、`POST /api/me/totp/disable {code}`
- JWT claims：`sub`(user id)、`role`、`stage`（`full`／`pending_totp`）

- [ ] Step 1: 測試（httptest + 真 MySQL）：`TestLoginWithoutTOTP`、`TestLoginWithTOTPRequiresCode`、`TestWrongTOTPRejected`、`TestPendingTokenCannotAccessMe`（Review Focus 5，預期 401）、`TestUserCannotAccessAdmin`（403）、`TestDisabledUserLoginRejected`、`TestWrongPassword401`
- [ ] Step 2: 跑 → FAIL；實作；跑 → PASS
- [ ] Step 3: commit `feat: auth with password, totp and roles`

### Task 4: API Key 與限流

**Files:** Create `backend/internal/apikey/{key.go,handlers.go,middleware.go,apikey_test.go}`, `backend/internal/ratelimit/{limiter.go,limiter_test.go}`

**Interfaces:**
- Consumes: `auth.RequireUser()`
- Produces: `apikey.Generate() (plain, hash, prefix string)`、`apikey.Authenticate(db) gin.HandlerFunc`（讀 `Authorization: Bearer sk-...`，把 key 與 user 放進 context）、`ratelimit.New(rdb *redis.Client) *Limiter`、`(*Limiter).Allow(ctx, keyID uint, rpm int) (bool, error)`、`ratelimit.Middleware(l *Limiter) gin.HandlerFunc`
- 路由：`GET /api/keys`、`POST /api/keys {name}` → 回 `{id,name,key}`（明文只此一次）、`PATCH /api/keys/:id {disabled}`（只能動自己的 key，別人的回 404）

- [ ] Step 1: 測試：`TestLimiterBlocksAfterRPM`（用 miniredis，rpm=3，第 4 次 false）、`TestLimiterResetsNextMinute`、`TestDisabledKey401`、`TestDisabledUserKey401`（Review Focus 4）、`TestCannotDisableOthersKey404`、`TestKeyStoredHashed`（DB 裡找不到明文）
- [ ] Step 2: 跑 → FAIL；實作；跑 → PASS
- [ ] Step 3: commit `feat: api keys and per-key rate limiting`

### Task 5: 轉發與 failover

**Files:** Create `backend/internal/relay/{types.go,openai.go,anthropic.go,router.go,relay_test.go}`

**Interfaces — Produces:**
- `relay.ChatRequest`／`relay.ChatResponse`（OpenAI 格式子集：model、messages[{role,content string}]、max_tokens、temperature、stream；response 含 usage）
- `type Adapter interface { Do(ctx, ch store.Channel, upstreamModel string, req ChatRequest) (ChatResponse, error) }`；`OpenAIAdapter`、`AnthropicAdapter`（system 訊息抽成 `system` 欄位、`max_tokens` 預設 1024、`stop_reason` → `finish_reason` 對應 `end_turn→stop`、`max_tokens→length`）
- `type UpstreamError struct{ Status int; Retryable bool; Body string }`
- `relay.Router{DB, Adapters map[string]Adapter}`；`(*Router).Dispatch(ctx, req) (resp ChatResponse, ch store.Channel, failovers int, err error)`：查 enabled 且 models 含 req.Model 的通路依 priority 排序，套 model_map，Retryable 錯誤換下一個

- [ ] Step 1: 測試（httptest 假上游）：`TestAnthropicRequestConversion`（system 抽出、max_tokens 補預設）、`TestAnthropicResponseConversion`、`TestFailoverOn500`（第一通路 500 → 第二通路成功、failovers=1）、`TestFailoverOnTimeout`、`TestNoFailoverOn400`（Review Focus 2：第二通路收到 0 次請求，回 UpstreamError 400）、`TestAllChannelsDown`、`TestModelMapApplied`
- [ ] Step 2: 跑 → FAIL；實作；跑 → PASS
- [ ] Step 3: commit `feat: relay adapters with priority failover`

### Task 6: 計費 扣款 用量 儲值 與 /v1 端點

**Files:** Create `backend/internal/billing/{pricing.go,ledger.go,handlers.go,billing_test.go}`, `backend/internal/relay/handler.go`, `backend/internal/relay/handler_test.go`

**Interfaces:**
- Consumes: `apikey.Authenticate`、`ratelimit.Middleware`、`(*Router).Dispatch`
- Produces: `billing.Cost(p store.ModelPrice, prompt, completion int) int64`、`billing.Reserve(db, userID uint, amount int64) error`（不足回 `billing.ErrInsufficient`）、`billing.Settle(db, userID uint, reserved, actual int64) error`、`billing.Topup(db, userID uint, amount int64, source string) error`
- 路由：`POST /v1/chat/completions`、`GET /v1/models`（列有價格且有通路的模型）、`GET /api/billing/balance`、`POST /api/billing/topup {amount_usd}`、`GET /api/usage?from&to&page`（自己的 usage_logs + 合計）
- 沒設價格的模型回 400 `model_not_found`；`stream:true` 回 400

- [ ] Step 1: 測試：`TestCostRoundsUp`、`TestReserveInsufficient402`、`TestConcurrentReserveNeverNegative`（Review Focus 1：20 goroutine、餘額夠 5 次 → 恰 5 成功、餘額 ≥ 0）、`TestSettleRefundsDifference`、`TestAllUpstreamDownRefundsAndLogs502`（Review Focus 3）、`TestChatCompletionEndToEnd`（mock 上游 → 200、usage_logs 一筆、餘額減少剛好 Cost）、`TestTopupRange`（0 與 1001 回 400）
- [ ] Step 2: 跑 → FAIL；實作；跑 → PASS
- [ ] Step 3: commit `feat: prepaid billing, usage logs, topup and chat endpoint`

### Task 7: 管理 API

**Files:** Create `backend/internal/admin/{handlers.go,admin_test.go}`

**Interfaces — Produces（全部掛 `auth.RequireAdmin()`）:**
- `GET /api/admin/users`、`PATCH /api/admin/users/:id {role?,disabled?}`、`POST /api/admin/users/:id/credit {amount_usd}`（可負數，扣到負回 400）
- `GET/POST /api/admin/channels`、`PATCH/DELETE /api/admin/channels/:id`（回傳時 api_key 只顯示末 4 碼）
- `GET /api/admin/prices`、`PUT /api/admin/prices/:model {input_per_m,output_per_m}`、`DELETE /api/admin/prices/:model`
- `GET /api/admin/usage`（全站用量，可依 user 篩）
- seed：啟動時若 channels 為空，建兩個指向 mockupstream 的通路（OpenAI priority 1、Anthropic priority 2，model_map `gpt-4o-mini→claude-haiku-4-5`）與 `gpt-4o-mini` 價格

- [ ] Step 1: 測試：`TestAdminCreditAndAudit`（topups 有 source=admin 紀錄）、`TestChannelKeyMasked`、`TestAdminCannotDemoteSelf`（避免把自己鎖在外面，回 400）、`TestNonAdmin403`（每個 admin 路由都打一次）
- [ ] Step 2: 跑 → FAIL；實作；跑 → PASS
- [ ] Step 3: commit `feat: admin api for users, channels and prices`

### Task 8: 前端（控制台＋管理後台）

**Files:** Create `web/`（Vite React TS）、`web/src/api.ts`、`web/src/pages/*`、`web/nginx.conf`、`web/Dockerfile`

**Interfaces:** Consumes Task 3–7 所有 `/api/*` 路由
- 頁面：登入（含 TOTP 第二步）、註冊、總覽（餘額＋近 7 天用量表）、API Keys（建立後明文顯示一次＋複製、停用／啟用）、用量明細、儲值、安全性（TOTP 設定 顯示 QR code 用 `qrcode` 套件，輸入碼啟用）；admin 選單只有管理員看得到：使用者、通路（含「測試」按鈕不做，YAGNI）、價格、全站用量
- `api.ts`：所有呼叫檢查 `res.ok`，失敗丟出 `{status,message}`，頁面顯示紅色錯誤框（不藏起來）
- 手機寬 390px 版面可用（側邊選單收成上方選單）

- [ ] Step 1: `npm run build` 與 `tsc --noEmit` 通過
- [ ] Step 2: 開 `npm run dev` 用 Playwright MCP 手動走一次：註冊 → 開 TOTP → 登出再登入 → 建 key → curl 打 `/v1/chat/completions` → 用量頁看到該筆、餘額減少；截圖存 `docs/screenshots/`
- [ ] Step 3: commit `feat: developer console and admin dashboard`

### Task 9: Docker Compose OpenAPI README CI Playwright 驗收

**Files:** Create `backend/Dockerfile`, `mockupstream/Dockerfile`, 完整 `docker-compose.yml`, `docs/openapi.yaml`, `README.md`, `.github/workflows/ci.yml`, `e2e/{playwright.config.ts,gateway.spec.ts}`

**Interfaces:** OpenAPI 涵蓋 `/v1/chat/completions`、`/v1/models` 與全部 `/api/*`；後端 `GET /docs` 用 Swagger UI（CDN 固定版本）顯示 `docs/openapi.yaml`

- [ ] Step 1: 寫 e2e：`開發者完整流程`（註冊→儲值→建 key→API 呼叫→用量出現）、`TOTP 登入`（用 otplib 算碼）、`failover`（`/_control` 讓 openai 500 → 呼叫仍 200 且用量頁通路顯示 Anthropic）、`餘額不足 402`、`一般使用者打 admin API 403 且畫面無管理選單`；config 設 `failOnFlakyTests: true`
- [ ] Step 2: `docker compose up -d --build` 一鍵起（mysql、redis、mockupstream、gateway、web），`curl localhost:8080/healthz` 回 ok，跑 `npx playwright test --repeat-each=3` 全綠
- [ ] Step 3: CI：job `backend`（mysql+redis services，`go vet`、`go test -race ./...`）、job `mockupstream`、job `web`（build+tsc）、job `security`（gosec、semgrep）；Playwright e2e 只在本機跑（Steven 2026-11-19 定）
- [ ] Step 4: README：架構圖（mermaid）、一鍵啟動、demo 腳本（含 curl 範例和 failover 示範指令）、預設帳號、設計重點（預扣結算、failover 規則、金額整數）
- [ ] Step 5: commit `feat: compose, openapi, ci and e2e acceptance`

### Task 10: 安全稽核

**Files:** Create `docs/security-audit.md`；修改稽核找到的問題所在檔案

- [ ] Step 1: 裝 gosec（`go install github.com/securego/gosec/v2/cmd/gosec@latest`）、semgrep（`python3 -m pip install semgrep`）
- [ ] Step 2: `gosec -fmt=json -out=docs/audit/gosec.json ./...`；`semgrep --config p/golang --config p/react --config p/owasp-top-ten --config p/secrets --json -o docs/audit/semgrep.json .`
- [ ] Step 3: 每條 finding 判定 真問題（修）／誤報（寫理由，必要時 `#nosec` 附原因）；修完重跑，真問題歸零
- [ ] Step 4: 報告一頁：工具版本、規則集、發現數（修前／修後）、每條處置、另外人工檢查的項目（金額整數、Key 雜湊、cookie 屬性、限流 fail-open 取捨）
- [ ] Step 5: commit `docs: security audit report`

### Task 11: 推上 GitHub 驗 CI

- [ ] Step 1: `gh auth switch -u Chunweiwu518`；`gh repo create Chunweiwu518/llm-gateway --private --source . --push`；做完切回原本的 active 帳號 `wuchunwei05180701`
- [ ] Step 2: `gh run watch` 等 CI，全部 job 綠才算完成；紅的修到綠
- [ ] Step 3: 回報 repo 網址、demo 步驟、履歷重點三到五行

## 驗收條件（怎麼操作 → 應該看到什麼）

| 操作 | 應該看到 |
|---|---|
| `cp .env.example .env && docker compose up -d --build` | 五個容器 healthy，`http://localhost:5173` 出現登入頁 |
| 用 README 的 curl 打 `/v1/chat/completions` | 200，回 `[mock-openai] ...`，控制台用量多一筆、餘額減少 |
| `curl -X POST localhost:9000/_control -d '{"provider":"openai","mode":"500"}'` 後再打 | 仍 200，回 `[mock-anthropic] ...`，用量顯示 failover 1 次 |
| 管理員把某人餘額調到 0 後他再打 | 402，OpenAI 錯誤格式 |
| 同一把 key 一分鐘打第 61 次 | 429 |
| GitHub Actions | backend、mockupstream、web、security 全綠 |
| 本機 `npx playwright test` | 五條 e2e 全綠 |
