# 重構說明：從 pgx/sqlc 遷移至 GORM

**Commit**: `ce929a6`  
**日期**: 2026-01-28  
**類型**: BREAKING CHANGE

---

## 背景

原本的 persistence layer 使用 **pgx**（原生 PostgreSQL driver）搭配 **sqlc**（SQL 自動產生程式碼）。這次重構將整層替換為 **GORM**，以簡化 schema 管理、統一錯誤處理、並讓整個 codebase 更容易維護。

---

## 主要變動

### 1. 資料庫連線層（`internal/database/`）

新增 `gorm.go`，取代原本散落在 repository 的 `connection.go`：

- 使用 `gorm.Open(postgres.Open(...))` 建立連線
- 設定 `SingularTable: true`（表名不自動加 s）
- Connection pool 設定：MaxOpenConns=50、MaxIdleConns=10、ConnMaxLifetime=1h

```go
// 舊
conn, _ := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))

// 新
db, _ := database.NewGormDB(os.Getenv("DATABASE_URL"))
```

---

### 2. Domain Models（`internal/domain/`）

所有 model 從 pgx 的 `db:` tag 改為 GORM 的 `gorm:` tag，ID 類型從 `int` 全面改為 `uuid.UUID`。

#### User

| 欄位 | 變更 |
|------|------|
| `user_id int` | `ID uuid.UUID` (primaryKey) |
| 新增 | `Email`, `AvatarURL`, `Timezone`, `LateStreak`, `Points`, `Level` |
| 新增 | `CreatedAt`, `DeletedAt`（支援 soft delete） |

#### Mistake

| 欄位 | 變更 |
|------|------|
| `mistake_id int` | `ID uuid.UUID` |
| `practice_id int` | `EventID uuid.UUID`（概念重命名） |
| `mistake_status string` | 移除 |
| 新增 | `OriginText`, `FixedText`, `Comment`, `Note` |
| `mistake_type string` | 改為 `MistakeType` enum（`grammar`/`vocab`/`pronounce`/`advanced`） |

#### Transcript

| 欄位 | 變更 |
|------|------|
| `transcript_id int` | `ID uuid.UUID` |
| `mistake_id int` | `EventID uuid.UUID` + 新增 `UserID` |
| `furigana string` | 移除 |
| `content string` | 改為 `Transcript string` |
| 新增 | `StartTime`, `EndTime`, `Note`, `Accent`（jsonb） |

#### 新增 Domain（全新）

- **`Event`**：取代原本的 `Practice`/`AICorrection`/`Note`。表示一場日文練習活動，帶有 `EventMode`（`report`/`conversation`/`discussion`/`review`）。
- **`EventAttendee`**：記錄 Event 的參與者。

---

### 3. Repository 層（`internal/repository/`）

**移除（pgx 實作）**：
- `postgres_ai_correction.go`
- `postgres_mistake.go`
- `postgres_note.go`
- `postgres_practice.go`
- `postgres_transcript.go`
- `postgres_user.go`
- `connection.go`

**新增（GORM 實作）**：
- `gorm_user.go`
- `gorm_event.go`
- `gorm_event_attendee.go`
- `gorm_transcript.go`
- `gorm_mistake.go`

**新增錯誤映射**（`errors.go`）：

```go
// GORM 錯誤統一映射為 domain 錯誤
func MapGormError(err error) error {
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return domain.ErrNotFound
    }
    if strings.Contains(err.Error(), "duplicate key") {
        return domain.ErrDuplicateEntry
    }
    return err
}
```

所有 repository 方法都必須在 return 前呼叫 `MapGormError(err)`。

**Repository 測試**：每個 GORM repository 均新增對應的 `_test.go`，使用 `go-sqlmock` 模擬資料庫。

---

### 4. API 層（`internal/api/`）

`API` struct 的依賴改為直接持有 `*gorm.DB`，並重新初始化 repositories：

```go
// 舊
func NewAPI(..., conn repository.Connection, ...) *API

// 新
func NewAPI(..., db *gorm.DB, ...) *API
```

**Route 變更**：

| 變更 | 說明 |
|------|------|
| 移除 `/ai-corrections` 路由群組 | 功能併入 Event/Mistake |
| 移除 `/notes` 路由群組 | 功能併入 Mistake.Note 欄位 |
| `GET /mistakes/practice/:id` | 改為 `GET /mistakes/event/:id` |

所有 handler 中的 ID 解析改用 `uuid.Parse()`，錯誤判斷改用 `errors.Is()`。

---

### 5. 啟動流程（`internal/cmd/api.go`）

```go
// 舊：手動建立 pgxpool，repositories 在 API constructor 內部初始化
dbpool, _ := pgxpool.New(...)

// 新：GORM 連線 + AutoMigrate，repositories 在 NewAPI 中注入
db, _ := database.NewGormDB(...)
db.AutoMigrate(&domain.User{}, &domain.Event{}, &domain.EventAttendee{}, &domain.Transcript{}, &domain.Mistake{})
```

AutoMigrate 在每次啟動時自動同步 schema，不再需要手動維護 SQL migration 檔案作為主要方式。

---

### 6. 基礎設施

| 新增項目 | 說明 |
|----------|------|
| `Dockerfile` | Multi-stage build（builder + 最小 runtime image） |
| `docker-compose.yml` | 本地開發環境（app + PostgreSQL） |
| `AGENTS.md` | Codebase 規範文件（供 AI 輔助開發使用） |
| CI workflow | golangci-lint + go build/test 自動化檢查 |

**移除**：`sqlc.yaml` 及相關 pgx 依賴。

---

## 環境變數異動（BREAKING）

| 變數 | 狀態 | 說明 |
|------|------|------|
| `DATABASE_URL` | 必填（格式不變） | PostgreSQL 連線字串 |
| `JWKS_URL` | **新增必填** | JWT 驗證用的 JWKS endpoint |
| `API_TOOLS_URL` | 必填（不變） | 外部 AI tools 服務位址 |
| `PORT` | 選填（不變） | 服務 port |

---

## 對開發者的影響

1. **新增 domain model** → 在 `internal/domain/` 定義 struct，加上 `gorm:` tag，然後把 `&domain.YourModel{}` 加進 `internal/cmd/api.go` 的 `AutoMigrate` 清單。
2. **新增 repository** → 建立 `gorm_xxx.go`，實作 domain 的 interface，所有錯誤通過 `MapGormError` 回傳。
3. **ID 類型全為 `uuid.UUID`**，不使用 `int` 或 `string`。
4. **pgx / sqlc 相關程式碼已完全移除**，請勿再引入。
