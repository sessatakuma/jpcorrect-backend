# Schema 對比問題紀錄

比對 `database-schema.md` 與實際 domain models 後發現的中高嚴重問題。
建立日期：2026-02-22

---

## ✅ 已解決

### 1. 殭屍 Domain 檔案（重構殘留物）— 已於 2026-02-22 刪除

下列三個檔案已確認刪除：`ai_correction.go`、`note.go`、`practice.go`。

---

### 2. `Transcript.Accent` 型別設計不明確 — 已於 2026-02-22 解決

將 `Accent` 從 `string` 改為 `datatypes.JSON`（`gorm.io/datatypes`），GORM 自動處理 jsonb 序列化，API 回傳時 `accent` 為正確的 JSON 物件而非字串。格式未定義，待業務確認後可進一步收斂為具體 struct。

---

## 🟡 中嚴重

### 3. `Guild` 與 `Event` 缺少時間戳欄位

**現況**：

| Model | 缺少欄位 |
| ----- | -------- |
| `Guild` | `created_at`、`deleted_at` |
| `Event` | `created_at` |

`User` 有完整的 `CreatedAt` + `DeletedAt`（soft delete），但 `Guild` 和 `Event` 沒有。Schema 文件也未定義這些欄位，無法確認是刻意省略還是遺漏。

**潛在問題**：
- 無法知道公會或活動何時被建立
- `Guild` 沒有 soft delete，刪除會直接從資料庫移除（`GuildAttendee` 等關聯紀錄可能孤立）
- `Event` 同理，刪除後 `Transcript`、`Mistake`、`EventAttendee` 的 FK 會懸空

**建議**：確認業務需求後決定是否補上。若需要 soft delete，加上 `gorm.DeletedAt` 並加入 `AutoMigrate`。

---

### 4. `EventAttendee` / `GuildAttendee` 使用 `uniqueIndex` 的業務疑問

**現況**：
```go
// EventAttendee
EventID uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_event_user"`
UserID  uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_event_user"`

// GuildAttendee
GuildID uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_guild_user"`
UserID  uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_guild_user"`
```

複合 unique index 代表：**同一個 user 在同一場 event / 同一個公會只能有一筆紀錄**。

**問題**：若有「離開後重新加入」的需求，目前設計無法支援，因為：
1. 第一筆紀錄的 `leaved_at` 填入後，該 `(event_id, user_id)` 組合已佔用
2. 無法再建立新的加入紀錄

Schema 文件中有 `joined_at` / `leaved_at` 欄位暗示可能有重新加入的需求，但未明確說明。

**建議**：確認業務規則：
- 若**不允許**重新加入 → 現況正確，uniqueIndex 合理
- 若**允許**重新加入 → 應改為一般 index（`index:idx_event_user`），並調整查詢邏輯（以 `leaved_at IS NULL` 找當前成員）
