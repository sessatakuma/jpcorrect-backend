# 重構摘要：pgx/sqlc → GORM

**Commit**: `ce929a6` (2026-01-28)  
**類型**: BREAKING CHANGE

---

## 概述

將 persistence layer 從 pgx/sqlc（原生 SQL + code gen）遷移至 GORM ORM，簡化 schema 管理、統一錯誤處理。

---

## 關鍵變動

### ID 型別
- 從 `int` 改為 `uuid.UUID`（所有 PK/FK）

### Schema 管理
- 手動 SQL migrations → GORM AutoMigrate
- Schema 由 domain models 驅動（`internal/cmd/api.go`）

### Repository 層
- 移除 `postgres_*.go`（pgx 實作）
- 新增 `gorm_*.go`（GORM 實作）
- 新增 `MapGormError()` 統一錯誤映射
- 新增完整測試覆蓋（`gorm_*_test.go`）

### Domain Models 重構
- `Practice` → `Event`（更完整，含 `EventMode` enum）
- `AICorrection` → 併入 `Mistake.FixedText` / `Mistake.Comment`
- `Note` → 併入 `Transcript.Note` / `Mistake.Note`
- 新增 `EventAttendee`（多對多關係）
- `User` 擴充：Email, AvatarURL, PasswordHash, IsEmailVerified, Role, Status, Timezone, LateStreak, Points, Level, CreatedAt, UpdatedAt, DeletedAt

### API 變更
- 移除 `/ai-corrections`、`/notes` 路由
- `GET /mistakes/practice/:id` → `GET /mistakes/event/:id`

---

## 遷移後結構

```
internal/
  ├── domain/          # Business entities + repository interfaces
  ├── repository/      # GORM implementations + MapGormError()
  ├── database/        # GORM connection factory (gorm.go)
  └── api/            # HTTP handlers (now uses *gorm.DB)
```

---

## 完整 Schema 與設計

**請參閱** `docs/database-design.md` 取得完整的 schema 詳細、ERD 圖與 Developer Notes。

---

## 新增依賴

```
gorm.io/gorm
gorm.io/driver/postgres
github.com/google/uuid
```

## 移除依賴

```
github.com/jackc/pgx/v5
github.com/sqlc-dev/sqlc
```
