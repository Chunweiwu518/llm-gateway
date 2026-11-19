# 安全稽核報告

日期：2026-11-19
範圍：`backend/`、`mockupstream/`、`web/`、`.github/workflows/`、Dockerfile 與 nginx 設定

## 工具與規則

| 工具 | 版本 | 設定 |
|---|---|---|
| gosec | v2.30.0（Go 1.27.4） | 預設全部規則，`backend/` 與 `mockupstream/` 各掃一次 |
| Semgrep | 1.142.0（Docker image `semgrep/semgrep:1.142.0`） | `p/golang`、`p/react`、`p/typescript`、`p/owasp-top-ten`、`p/secrets`、`p/dockerfile`，共 346 條規則、86 個檔案 |

原始輸出在 `docs/audit/`，`*-before.json` 是修正前，`*-after.json` 是修正後。CI 的 `security` job 每次 push 都跑同一組設定，有新發現就失敗。

## 結果

| 工具 | 修正前 | 修正後 |
|---|---|---|
| gosec | 2 | 0（2 條判定誤報，以 `#nosec` 標註原因） |
| Semgrep | 11 | 0 |

### 工具發現與處置

| # | 工具／規則 | 位置 | 判定 | 處置 |
|---|---|---|---|---|
| 1 | Semgrep `github-actions-mutable-action-tag`（10 處） | `.github/workflows/ci.yml` | 真問題。`@v4` 這類 tag 可被改指到別的 commit，屬供應鏈風險 | 全部改為固定 commit SHA，tag 寫在註解 |
| 2 | Semgrep `request-host-used` | `web/nginx.conf` | 真問題，風險低。nginx 把用戶端的 Host 傳給後端，後端若用 Host 組網址會被注入 | 移除 `proxy_set_header Host $host`，後端收到固定的 upstream 名稱 |
| 3 | gosec G101 hardcoded credentials | `backend/internal/store/seed.go` | 誤報。字串是內建假上游的佔位 key，假上游不驗證內容 | 加 `#nosec G101` 與原因 |
| 4 | gosec G706 log injection | `mockupstream/main.go` | 誤報，風險低。值來自部署者設定的 `LISTEN_ADDR` | 改用 `%q` 輸出（換行會被跳脫），加 `#nosec G706` 與原因 |

### 人工檢查新增的問題

| # | 問題 | 處置 | 驗證 |
|---|---|---|---|
| 5 | 登入與 TOTP 驗證沒有嘗試次數上限，可暴力猜密碼或 6 位數驗證碼 | 用 Redis 計數：同一 email 每分鐘最多 10 次登入、同一帳號每分鐘最多 5 次 TOTP 驗證，超過回 429 | `TestLoginAttemptsLimited`、`TestTOTPAttemptsLimited` |
| 6 | 請求 body 沒有大小上限，大型 JSON 會被整個讀進記憶體 | 全部路由套 `http.MaxBytesReader`，上限 1 MiB | `TestLimitBodyRejectsLargeJSON` |

| 7 | `max_tokens` 沒有上限，極大值讓成本計算溢位成負數，預扣變成加錢 | `max_tokens` 限 1 到 65536；`Reserve` 拒絕負數金額 | `TestHugeMaxTokensRejected`、`TestReserveRejectsNegativeAmount` |
| 8 | 上游實際輸出超過預扣時補扣不設限，併發請求可讓餘額變負 | 沒帶 `max_tokens` 時把預扣用的 1024 轉給上游；結算補扣最多扣到餘額歸零 | `TestReservedMaxTokensForwarded`、`TestSettleNeverOverdraws` |

以上各項都先寫測試、看到失敗再修。第 7、8 項是整體程式碼審查找到的。

## 人工檢查清單

| 項目 | 做法 | 結果 |
|---|---|---|
| 密碼儲存 | bcrypt（cost 10），長度 8 到 72；查無帳號時比對假雜湊，回應時間不洩漏帳號是否存在 | 通過 |
| Session | JWT HS256，驗證時限定演算法；secret 至少 32 字元，未設定時服務不啟動；cookie 為 httpOnly、SameSite=Lax、path `/api`，`COOKIE_SECURE=true` 時加 Secure | 通過 |
| 兩步驟驗證 | 密碼正確後只發 5 分鐘的 `pending_totp` token，這個 token 不能當 session 用 | 通過（`TestPendingTokenCannotAccessMe`） |
| CSRF | 會改資料的 API 都是 POST／PATCH／PUT／DELETE，SameSite=Lax 讓跨站請求不帶 cookie | 通過 |
| API Key | `crypto/rand` 產生 288 bits，資料庫只存 SHA-256；列表與管理畫面只顯示前 8 碼 | 通過（`TestKeyStoredHashed`） |
| 存取控制 | Key 操作限本人（他人的 Key 回 404）；管理 API 全部經過 `RequireAdmin`；管理員不能降級或停用自己 | 通過（`TestNonAdmin403` 逐一打 11 個管理路由） |
| 計費正確性 | 金額用 int64 micro-USD；預扣用條件式 UPDATE，併發不會超扣 | 通過（`TestConcurrentReserveNeverNegative`） |
| 上游憑證 | 通路 API key 不出現在任何 API 回應，管理畫面只顯示末 4 碼；更新通路時空白代表不變 | 通過（`TestChannelKeyMasked`） |
| SQL injection | 全部經 GORM 參數化查詢，沒有字串拼接 SQL | 通過 |
| 上游回應 | 讀取上限 4 MiB；錯誤內容截斷到 500 字元 | 通過 |
| 機密管理 | `.env` 不進版控；`.env.example` 只有佔位值；compose 缺 `JWT_SECRET` 或 `ADMIN_PASSWORD` 時拒絕啟動；測試與 CI 不使用真的模型 key | 通過 |
| 容器 | gateway、mockupstream、web 都以非 root 使用者執行；MySQL、Redis 等連接埠只綁 127.0.0.1 | 通過 |

## 已知限制與取捨

| 項目 | 說明 |
|---|---|
| 限流 fail-open | Redis 無法連線時放行請求並記 log。優先維持服務可用；需要嚴格限制時改為 fail-closed |
| 通路 base_url 可指向任意位址 | 只有管理員能設定，視為受信任操作；對外開放多租戶時需加上目的地白名單，避免 SSRF |
| 實際用量超過預扣時 | 只補扣到餘額歸零，差額記 log，由平台吸收 |
| Swagger UI 從 CDN 載入 | 固定版本號，但沒有加 Subresource Integrity |
| 上游 4xx 錯誤內容會回給呼叫端 | 方便除錯；內容已截斷，不含通路金鑰 |
