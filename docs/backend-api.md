# 後端 API 實現清單

## 0. 全域約定

### 0.1 Route convention

遵循既有 jpcorrect-backend pattern:

* 所有業務 endpoint 在 `/v1/{resource}` 之下
* Handler 命名:`{Resource}{Action}Handler`,例如 `ActivityCreateHandler`、`MistakeGetByEventHandler`
* HTTP method 對 CRUD:`POST` create / `GET` read / `PUT` update / `DELETE` delete
* `204 No Content` 用 `c.Status()` 不用 `c.JSON()`
* Error mapping(沿用既有 pattern):
  * `domain.ErrNotFound` → 404
  * `domain.ErrDuplicateEntry` → 409
  * `domain.ErrHasRelatedRecords` → 409
  * 其他 → 500

### 0.2 認證

* Supabase Auth 簽 JWT,前端 `Authorization: Bearer <jwt>` 傳給 backend
* 既有 `AuthMiddleware`(`internal/api/auth.go`)沿用,透過 JWKS 驗證
* JWT 的 `sub` claim 為 Supabase Auth user UUID
  * backend User 表的 `supabase_user_id` 欄位做 mapping(見 [Schema Plan sec 7.4](/doc/talkuma-schema-plan-BFikoV3J4K#h-74-%E6%98%AF%E5%90%A6%E9%9C%80%E8%A6%81-supabaseuserid-%E6%AC%84%E4%BD%8D))
* Middleware 將 backend [User.id](http://User.id) 寫入 `c.Get("userID")` 供 handler 使用

### 0.3 權限驗證

定義在 Schema Plan §4 的 helper functions。每個 endpoint 標明所需權限:

| 標記  | 意義  | helper |
|-----|-----|--------|
| 🔒  | 登入  | AuthMiddleware(JWT) |
| 🏰  | 公會成員 | RequireGuildMember |
| 👑  | 公會會長 | RequireGuildLeader |
| 🎯  | 活動參與 | RequireActivityMember(該 Activity 屬於 caller 的公會) |
| 👤  | 本人  | RequireSelfOrGuildLeader / RequireMistakeOwner |

### 0.4 Worker(Goroutine)

MVP 階段所有非同步任務跑在同一個 backend process,goroutine + channel 排隊。需要實作的 worker:

| Worker | 觸發  | 用途  |
|--------|-----|-----|
| `RecordingProcessingWorker` | 練習結束時 enqueue | 將 raw 多軌音訊整合+畫面制作 (podcast 風格) + 上傳 YouTube unlisted + 寫 youtube_url + 排程 raw 清除 |
| `AIAnalysisWorker` | 練習結束時 enqueue | 觸發 AI 初版分析<br>(MVP 人工替代,介面相同) |
| `NotificationDispatcher` | 業務事件觸發 | 寫 Notification 表 + WebSocket broadcast + 呼叫 push/email service |
| `WebPushSender` | NotificationDispatcher 呼叫 | 送 VAPID payload |
| `EmailSender` | NotificationDispatcher 呼叫 | 串 Resend / SendGrid |
| `Cron tasks` (下文 [§13.2](https://docs.sessatakuma.dev/doc/talkuma-api-V2NUJaPVrG#h-132-cron-任務清單)) | 時間觸發 | 各種定時掃描 |

### 0.5 Cron 排程

使用 robfig/cron 或 ticker pattern 在 backend process 內排程。具體任務見 [§13.2](https://docs.sessatakuma.dev/doc/talkuma-api-V2NUJaPVrG#h-132-cron-任務清單)。

### 0.6 MoSCoW

**M** = MVP 必要、**S** = MVP Should、**C** = 之後


---

## 1. 認證與帳號(M1)

### 1.1 OAuth 流程

OAuth callback 流程由 Supabase Auth 處理,backend 不接 callback。前端完成 OAuth 後拿到 JWT,呼叫 backend 確認 / 補資料。

| Method | Path | Handler | 權限  | 用途  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/v1/users/init` | `*`     | 🔒  | First-login 用,以 JWT 中的 supabase_user_id + email 建立 User row(若已存在則 idempotent return)。回傳完整 User profile | M      |
| GET    | `/v1/users/me` | `UserMeHandler` | 🔒  | 取目前登入者 profile | M      |
| PUT    | `/v1/users/me` | `UserMeUpdateHandler` | 🔒  | 更新暱稱、頭像 | M      |

### 1.2 既有 User endpoint(沿用,僅補充)

| Method | Path | Handler | 權限  | 備註  |
|--------|------|---------|-----|-----|
| GET    | `/v1/users/:id` | `UserGetHandler` | 🔒  | 取他人公開 profile;backend 過濾敏感欄位(email、status 等) |
| GET    | `/v1/users/name/:name` | `UserGetByNameHandler` | 🔒  | 既有,沿用 |
| GET    | `/v1/users/email/:email` | `UserGetByEmailHandler` | 🔒  | 既有,沿用 |

### 1.3 邀請接受(從 P-8 邀請落地頁)

| Method | Path | Handler | 權限  | 用途  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/invites/:token` | `InviteInfoHandler` | 公開(無需 JWT) | 取邀請連結資訊(公會名稱、簡介、是否過期),供 P-8 顯示 | M      |
| POST   | `/v1/invites/:token/accept` | `InviteAcceptHandler` | 🔒  | 已登入者接受邀請。檢查 token 有效、加入公會數 < 3、未在該公會 → 加入 + 取消其他 pending JoinRequest | M      |


---

## 2. Dashboard(P-4)

| Method | Path | Handler | 權限  | 用途  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/me/dashboard` | `MeDashboardHandler` | 🔒  | 跨公會待處理活動 + 我的公會清單 + 個人統計。內部 join Activity / FeedbackProgress / GuildAttendee。排序邏輯依 Dashboard Page Spec | M      |

回應結構:

```json
{
  "pending_activities": [
    {
      "activity_id": "...",
      "guild_id": "...",
      "guild_name": "...",
      "sequence_number": 3,
      "status": "in_feedback",
      "mode": "conversation",
      "practice_at": "2026-04-13T20:00:00Z",
      "review_at": "2026-04-20T20:00:00Z",
      "action_label": "繼續回饋",
      "action_target": "/feedback/{activity_id}"
    }
  ],
  "my_guilds": [
    { "guild_id": "...", "name": "...", "member_count": 4, "my_role": "leader" }
  ],
  "stats": {
    "joined_at": "2026-03-01T00:00:00Z",
    "guild_count": 2,
    "activity_count": 7
  }
}
```


---

## 3. 公會管理(M2)

### 3.1 公會基本

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/v1/guilds` | `GuildCreateHandler` | 🔒  | Caller 已建立公會數 < 1 才允許;同 transaction INSERT Guild + GuildAttendee(role=master) | M      |
| GET    | `/v1/guilds/:id` | `GuildGetHandler` | 🏰  | 公會基本資訊 | M      |
| PUT    | `/v1/guilds/:id` | `GuildUpdateHandler` | 👑  | 編輯名稱、簡介 | M      |
| DELETE | `/v1/guilds/:id` | `GuildDeleteHandler` | 👑  | (MVP S7-D3 不處理停滯公會,但保留 endpoint) | C      |
| POST   | `/v1/guilds/:id/transfer-leader` | `GuildTransferLeaderHandler` | 👑  | body: `{ new_leader_user_id }`。同 transaction 更新 GuildAttendee.role + Guild.leader | S      |

### 3.2 成員管理

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/guilds/:id/members` | `GuildMembersHandler` | 🏰  | 成員列表(含 role、joined_at) | M      |
| DELETE | `/v1/guilds/:id/members/:user_id` | `GuildMemberRemoveHandler` | 👑  | 移除成員。target ≠ self;觸發通知 | M      |
| POST   | `/v1/guilds/:id/leave` | `GuildLeaveHandler` | 🏰  | 自行退出。leader 必須先轉讓 | M      |

### 3.3 邀請連結

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/guilds/:id/invite-link` | `GuildInviteLinkGetHandler` | 👑  | 取目前有效 link;若已過期或無 row 則 200 + null | M      |
| POST   | `/v1/guilds/:id/invite-link` | `GuildInviteLinkCreateHandler` | 👑  | 產生新 link;同 transaction 將舊 link expires_at 設為 now() | M      |

### 3.4 加入申請(公會板路徑)

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/v1/guilds/:id/applications` | `GuildApplicationCreateHandler` | 🔒  | 送出加入申請;檢查 caller 不是該公會成員 + 未有 pending 申請 | S      |
| GET    | `/v1/guilds/:id/applications` | `GuildApplicationsHandler` | 👑  | 查公會 pending 申請 | S      |
| POST   | `/v1/guilds/:id/applications/:app_id/approve` | `GuildApplicationApproveHandler` | 👑  | 通過。同 transaction 加入 + cancel caller 其他 pending | S      |
| POST   | `/v1/guilds/:id/applications/:app_id/reject` | `GuildApplicationRejectHandler` | 👑  | 拒絕  | S      |

### 3.5 預設時段

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/guilds/:id/default-slot` | `GuildDefaultSlotGetHandler` | 🏰  | 取預設時段(可能為 null) | M      |
| PUT    | `/v1/guilds/:id/default-slot` | `GuildDefaultSlotUpsertHandler` | 👑  | 設定 / 更新 | M      |
| DELETE | `/v1/guilds/:id/default-slot` | `GuildDefaultSlotDeleteHandler` | 👑  | 取消預設 | M      |

### 3.6 公會板

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/guilds/discover` | `GuildDiscoverHandler` | 🔒  | 公開公會列表(分頁,按最近活動時間倒序) | S      |


---

## 4. 活動 / 排程(M3)

> **核心抽象:Activity = 一場練習 + 一場復盤的配對**。詳見 Schema Plan [§2.1](https://docs.sessatakuma.dev/doc/talkuma-schema-plan-BFikoV3J4K#h-21-activity配對單位)。

### 4.1 Activity CRUD

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/v1/guilds/:id/activities` | `ActivityCreateHandler` | 👑  | 排定新活動。body: `{ practice_at, mode, theme?, announce_topic_id?, review_at?, reporter_user_ids? }`。同 transaction 建 Activity + 兩個 Event(practice + review)。`review_at` 預設 +7 天。觸發通知 | M      |
| GET    | `/v1/activities/:id` | `ActivityGetHandler` | 🎯  | 含 Event(practice/review)、status、mode、theme、youtube_url、ai_status | M      |
| PUT    | `/v1/activities/:id` | `ActivityUpdateHandler` | 👑  | 調整時間、模式、主題、報告者。觸發通知 | M      |
| POST   | `/v1/activities/:id/abort` | `ActivityAbortHandler` | 👑  | 中止。body: `{ reason? }`。寫 aborted_by/aborted_at/aborted_reason。觸發通知 | M      |
| GET    | `/v1/guilds/:id/activities` | `GuildActivitiesHandler` | 🏰  | 公會活動列表(分頁無限滾動 20 筆,UF-T15)。query param `status` 可選 filter(動態牆要近期、歷史記錄要 ended) | M      |

### 4.2 報告練習輪值建議

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/guilds/:id/report-rotation-suggestion` | `GuildReportRotationHandler` | 👑  | 排程 modal 用。回傳成員 + 上次報告活動 + 系統預選的 2 人(最久未報告) | M      |

### 4.3 既有 `/v1/practices` 的處理

既有 `/v1/practices/*` 路由(架構圖 orange 部分)是早期 Practice → Event 的 backward compat。

**建議方向**:**保留但 deprecate**。MVP 前端不再使用,改用 `/v1/activities/*`。Backend 保留 endpoint 給後台 / debug 用。長期可移除。


---

## 5. 動態牆(M10)

### 5.1 貼文與留言

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/guilds/:id/feed` | `GuildFeedHandler` | 🏰  | 動態牆貼文列表(分頁無限滾動 20 筆) | M      |
| POST   | `/v1/guilds/:id/feed/announcements` | `GuildAnnouncementCreateHandler` | 👑  | 發公告 | M      |
| DELETE | `/v1/feed/posts/:post_id` | `FeedPostDeleteHandler` | 👑  | 刪自己的公告 / leader 刪任意 | M      |
| GET    | `/v1/feed/posts/:post_id/comments` | `FeedCommentsHandler` | 🏰  | 留言(分頁) | M      |
| POST   | `/v1/feed/posts/:post_id/comments` | `FeedCommentCreateHandler` | 🏰  | 留言  | M      |
| DELETE | `/v1/feed/comments/:comment_id` | `FeedCommentDeleteHandler` | 👑  |     | M      |

> 📝 系統貼文(`type=system`)由通知派送 worker 自動寫入,沒有對應的 create endpoint。

### 5.2 動態牆 Realtime 推送

走 WebSocket Hub。Channel:`feed:{guild_id}`。

WebSocket message:`feed-post-created`、`feed-comment-created`、`feed-post-deleted`、`feed-comment-deleted`。


---

## 6. 通知系統

### 6.1 站內通知

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/me/notifications` | `MeNotificationsHandler` | 🔒  | 通知列表(分頁,按時間倒序,含已讀狀態 + deeplink) | M      |
| GET    | `/v1/me/notifications/unread-count` | `MeUnreadCountHandler` | 🔒  | 未讀數(鈴鐺 badge 用) | M      |
| POST   | `/v1/me/notifications/:id/read` | `NotificationReadHandler` | 🔒  | 標單則已讀 | M      |
| POST   | `/v1/me/notifications/mark-all-read` | `NotificationMarkAllReadHandler` | 🔒  | 全部已讀 | M      |

### 6.2 Web Push 訂閱

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/v1/me/push-subscriptions` | `PushSubscriptionCreateHandler` | 🔒  | 註冊 subscription | M      |
| DELETE | `/v1/me/push-subscriptions/:id` | `PushSubscriptionDeleteHandler` | 🔒  | 取消訂閱 | M      |

### 6.3 通知派送邏輯(NotificationDispatcher worker)

業務事件觸發後,worker 依 Design Spec 3.2 表決定走哪些管道:

* 站內:寫 Notification 表 + WebSocket broadcast 到 `notifications:{user_id}` channel
* Web Push:依使用者訂閱送 VAPID payload
* Email:呼叫 EmailSender

事件清單(對應 Design Spec 3.2):

```
activity.scheduled
activity.updated
activity.aborted
activity.starting_soon          (cron 觸發)
analysis.completed
youtube.upload_completed         (S — 通知補充,可空)
feedback.deadline_approaching   (cron 觸發)
application.submitted
application.approved
application.rejected
member.joined
member.removed
feed.announcement_posted
feed.comment_posted
inactivity.no_next_activity     (cron 觸發)
inactivity.invite_link_expiring (cron 觸發)
inactivity.no_member_joined     (cron 觸發)
```

### 6.4 即時通知 WebSocket Channel

`notifications:{user_id}`。

Message:`notification-created`、`notification-read`(用於多裝置同步)。


---

## 7. 題庫(M7)

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/topics/:id` | `TopicGetHandler` | 🔒  | 取單一題目(含 hint vocab/grammar) | M      |
| GET    | `/v1/topics/announce/search` | `TopicAnnounceSearchHandler` | 🔒  | 排程 modal 公告組題目挑選用。query: `q?` keyword | M      |
| GET    | `/v1/topics/random` | `TopicRandomHandler` | 🔒  | 隨機抽抽選組題目;由 backend 在練習房間「題目確認」步驟由主持人推進時觸發 | M      |
| GET    | `/v1/topics/report-suggestions` | `ReportThemeSuggestionsHandler` | 🔒  | 報告主題建議清單 | S      |

> 📝 題庫內容 CRUD 由平台後台維運,不開公開 API(MVP)。


---

## 8. 練習房間(M4 + M5/M6)

### 8.1 進入前

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/v1/activities/:id/practice-room/eligibility` | `PracticeEligibilityCheckHandler` | 🎯  | 檢查活動非終態 + 簽 WebRTC 票券(若需要)。回傳 WebSocket connection token | M      |

### 8.2 練習進行中(WebSocket signaling)

走既有 `/ws` endpoint,沿用 Hub pattern。本節列出 Talkuma 新增的 message type(WebRTC offer/answer/ice 等不變):

| Message Type | Direction | 用途  |
|--------------|-----------|-----|
| `practice-join-room` | C→S       | 帶 activity_id 加入練習房間 channel |
| `practice-host-changed` | S→All     | 主持人變更廣播(含新主持人 user_id) |
| `practice-host-transfer` | C→S       | 主持人請求轉移給某成員(僅主持人可發) |
| `practice-step-advanced` | S→All     | 步驟推進(由主持人 client 觸發 → server 廣播) |
| `practice-step-next` | C→S       | 主持人按下一步 |
| `practice-step-time-update` | C→S       | 等待階段主持人微調步驟時間 |
| `practice-recording-state` | S→All     | 錄影開始 / 結束狀態變更 |
| `practice-start-recording` | C→S       | 主持人按開始練習 |
| `practice-stop-recording` | C→S       | 主持人按結束練習 |
| `practice-whiteboard-update` | S→All     | 共享白板增量更新(對話練習) |
| `practice-voice-state` | S→All     | 成員靜音 / 說話狀態變更 |

### 8.3 業務事件 → Repository 寫 DB(同 process function call)

WebRTC Hub 在以下時機**直接呼叫 repository**(同 process,不需 webhook):

| 事件  | 寫入操作 |
|-----|------|
| 第一個成員加入練習房間 | `ActivityRepo.UpdateStatus(id, "in_practice")` + 觸發 `activity.starting_soon` 通知(若還沒發過) |
| 主持人按開始練習 | `EventRepo.MarkRecordingStarted(eventID, hostUserID)` 寫 `recording_started_at` + `recording_started_by` |
| 主持人按結束練習 | 同 transaction:`EventRepo.MarkRecordingEnded(eventID)` + `ActivityRepo.UpdateStatus(id, "analyzing")` + `Worker.EnqueueRecordingProcessing(activityID)` + `Worker.EnqueueAIAnalysis(activityID)` + 觸發 `analysis.started` 通知(可選) |
| 步驟推進 | `EventRepo.UpdateCurrentStep(eventID, stepIndex)`(節流寫,用於斷線重連還原) |

> 📝 Step source of truth 是 Hub 內存,DB 只是 mirror。

### 8.4 個人筆記(報告練習)

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/activities/:id/practice-notes/me` | `PracticeNoteGetHandler` | 🎯  | 取自己的筆記 | M      |
| PUT    | `/v1/activities/:id/practice-notes/me` | `PracticeNoteUpsertHandler` | 🎯  | 自動儲存(debounced) | M      |

### 8.5 步驟時間微調(等待階段)

由 WebSocket message `practice-step-time-update` 觸發,由 Hub 寫 Event.step_durations(JSONB)。不另開 REST endpoint。


---

## 9. AI 初版分析(F-8.1)

### 9.1 AI Worker 內部介面

`AIAnalysisWorker` 是 goroutine,從 channel 拉任務後:

* MVP 階段:標記任務 `processing`,寫到後台介面待人工處理(實作為人工後台前端 + 一個內部 endpoint)
* 正式版:呼叫 AI service,等回傳結果

完成後寫入 transcript_lines(Transcript 表)+ Mistake 表 + 觸發 `ActivityRepo.MarkAIAnalysisCompleted(activityID)` → status `analyzing → in_feedback` → 發 `analysis.completed` 通知。

### 9.2 人工後台 endpoint(MVP 內測用,正式版 deprecate)

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/internal/ai-analysis/submit` | `AIAnalysisSubmitHandler` | shared secret(非 JWT) | 人工後台提交分析結果。body 含 transcript + mistakes + confidence。idempotent | M      |

> 📝 此 endpoint 設計需與正式版 AI service 介面**完全一致**(階段 0 / D4)。

### 9.3 讀取分析結果(已包在 §10/§11/§12 各場景)

不獨立 endpoint,透過 transcript / mistake / activity 各 endpoint 讀取。


---

## 10. AI Chatbot(M13)

### 10.1 會後回饋場景(P-22)

涉及 LLM 呼叫(OpenAI / Anthropic API),走 backend 處理。

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/v1/chatbot/feedback/sessions` | `ChatbotFeedbackStartHandler` | 🎯 + 👤(該 mistake 的 speaker) | body: `{ mistake_id }`。回傳 `{ session_id, opening_message }` | M      |
| POST   | `/v1/chatbot/feedback/sessions/:session_id/messages` | `ChatbotFeedbackMessageHandler` | session 擁有者 | SSE stream 回應 LLM | M      |
| POST   | `/v1/chatbot/feedback/sessions/:session_id/finalize` | `ChatbotFeedbackFinalizeHandler` | session 擁有者 | body: `{ accepted_correction }`。同 transaction:`MistakeRepo.UpdateLatestCorrection(...)` + 寫 ChatbotUsage | M      |

> 📝 Session state(對話歷史)放在 backend 的 in-memory map 或 Redis(不寫 DB,S7-D5)。Session id 過期清除。 📝 ChatbotUsage 在 finalize 時 + session 過期 timer 觸發時都要寫,確保不漏記。

### 10.2 復盤會場景(P-23)

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/v1/chatbot/review/lookup` | `ChatbotReviewLookupHandler` | 🎯  | 第一階段查證對話。body: `{ activity_id, current_mistake_id, message }`。回傳 LLM 回答(SSE)。共享 session(所有成員看到同一 session 結果) | M      |
| POST   | `/v1/chatbot/review/stage2` | `ChatbotReviewStage2Handler` | 🎯 + 主持人 | 第二階段歸納。body: `{ activity_id, instruction }`。LLM 回應 + 寫 ReviewRecord | M      |

> 📝 Review chatbot 對話結果 broadcast 給所有成員透過 WebSocket `review-chatbot-message`(見 §12)。

### 10.3 歷史記錄繼續對話(UF-D26)

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/v1/chatbot/historical/sessions` | `ChatbotHistoricalStartHandler` | 🎯 + 👤(speaker) | 從 P-13 對自己的句子重啟對話。其餘流程同 §10.1 | M      |


---

## 11. 會後回饋(M8)

### 11.1 取資料

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/activities/:id/transcript` | `ActivityTranscriptHandler` | 🎯  | 取整場逐字稿(每行:speaker、time、text、accent、mistake_id?) | M      |
| GET    | `/v1/activities/:id/mistakes` | `ActivityMistakesHandler` | 🎯 + 內容過濾 | 取 mistakes;**內容可見性過濾**:回饋期間僅自己可看自己的 latest_correction;復盤完成後成員可看全部 | M      |
| GET    | `/v1/mistakes/:id` | `MistakeGetHandler`(既有) | 同上  | 單一 mistake 詳情 | M      |
| PATCH  | `/v1/mistakes/:id/interacted` | `MistakeMarkInteractedHandler` | 👤(speaker) | 第一次點開時標 interacted_at | M      |

### 11.2 選題

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/me/activities/:id/selected-sentences` | `MySelectedSentencesHandler` | 🎯  | 取自己已選 | M      |
| POST   | `/v1/me/activities/:id/selected-sentences` | `SelectSentenceHandler` | 🎯 + 👤 | body: `{ mistake_id }`。檢查上限 ≤ 2 + mistake.speaker = self | M      |
| DELETE | `/v1/me/selected-sentences/:id` | `UnselectSentenceHandler` | 👤  | 取消勾選 | M      |

### 11.3 回饋進度

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/me/activities/:id/feedback-progress` | `MyFeedbackProgressHandler` | 🎯  | 取自己進度 | M      |
| POST   | `/v1/me/activities/:id/feedback-progress/start` | `FeedbackProgressStartHandler` | 🎯  | 第一次進入 P-22 時呼叫,寫 started_at + status=in_progress | M      |
| POST   | `/v1/me/activities/:id/feedback-progress/complete` | `FeedbackProgressCompleteHandler` | 🎯  | 完成回饋。檢查 ≥ 1 selected_sentence。寫 completed_at | M      |
| GET    | `/v1/activities/:id/feedback-progress/all` | `ActivityFeedbackProgressAllHandler` | 👑  | leader 看所有人進度(僅進度,不含內容) | M      |
| POST   | `/v1/activities/:id/feedback-progress/remind` | `FeedbackProgressRemindHandler` | 👑  | 手動提醒未完成成員 | S      |

### 11.4 降級機制(cron worker)

不暴露 endpoint。`cron-degrade-feedback` 每 5 分鐘掃描:

* 復盤會開始時間到 ∩ FeedbackProgress.status ≠ completed 的 (activity, user)
* 對該成員的 Mistake list 自動選最多 2 題(優先選 confidence=low),寫 SelectedSentence 標 `is_degraded=true`、`selected_by_user_id` 仍記原成員
* 若該成員 0 個 mistake,跳過(UF-5 邊界)
* Activity status: `in_feedback → in_review`
* 觸發 `feedback.deadline_passed` 通知


---

## 12. 復盤房間(M9)

### 12.1 進入

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/v1/activities/:id/review-room/eligibility` | `ReviewEligibilityCheckHandler` | 🎯  | 同 §8.1 概念 | M      |
| GET    | `/v1/activities/:id/review-discussion-list` | `ReviewDiscussionListHandler` | 🎯  | 復盤清單(按時間序排所有 SelectedSentence + 標 selector + degraded) | M      |

### 12.2 進行中(WebSocket)

新增 message type:

| Message Type | Direction | 用途  |
|--------------|-----------|-----|
| `review-join-room` | C→S       | 加入復盤 channel |
| `review-host-changed` | S→All     | 主持人變更 |
| `review-host-transfer` | C→S       | 主持人轉移 |
| `review-discussion-jump` | C→S       | 主持人切換到第 N 題 |
| `review-discussion-jumped` | S→All     | 廣播跳題(所有人同步) |
| `review-correction-reveal` | C→S       | 主持人按公布解析 |
| `review-correction-revealed` | S→All     | 廣播解析揭曉 |
| `review-stage-enter-2` | C→S       | 主持人進入第二階段 |
| `review-stage-changed` | S→All     | 廣播階段變更 |
| `review-chatbot-message` | S→All     | 復盤共享 chatbot 對話訊息廣播(由 §10.2 endpoint 觸發後 server 廣播) |
| `review-pip-toggle` | C→S       | 個別成員 PiP 切換(無需廣播,但記錄連線狀態) |

### 12.3 結束

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| POST   | `/v1/activities/:id/review-room/end` | `ReviewEndHandler` | 主持人 | 結束復盤。Activity status: `in_review → done`。若 leader 未排下次活動則 flag(前端取得後顯示提示) | M      |


---

## 13. 活動狀態機與 Cron

### 13.1 狀態轉換來源(同 process function call,非 endpoint)

```
pending_practice → in_practice    (Hub: 第一個成員加入)
in_practice      → analyzing      (Hub: 主持人按結束練習)
analyzing        → in_feedback    (AIAnalysisWorker: 分析完成。YouTube 慢來不影響)
in_feedback      → in_review      (cron-degrade-feedback: 復盤開始時間到 + 觸發降級)
in_review        → done           (ReviewEndHandler: 主持人結束)
*                → aborted        (ActivityAbortHandler 或 cron-stale-activity)
```

### 13.2 Cron 任務清單

放在 backend process 內,使用 ticker / robfig/cron:

| 任務  | 頻率  | 用途  |
|-----|-----|-----|
| `cron-activity-starting-soon` | 每 5 分鐘 | 練習開始 30 分鐘前發通知 |
| `cron-stale-activity` | 每小時 | 活動超過預定時間 N 小時無人加入 → abort |
| `cron-degrade-feedback` | 每 5 分鐘 | 復盤會開始時間到 → 觸發降級機制 + status 推進 |
| `cron-feedback-deadline-warn` | 每 6 小時 | 復盤前 24 小時提醒未完成回饋的成員 |
| `cron-invite-link-expiring` | 每天  | 邀請連結 1 天內到期 → 通知 leader |
| `cron-invite-link-cleanup` | 每天  | 過期連結標記(audit 用,不刪除) |
| `cron-no-next-activity` | 每天  | 復盤結束後 N 天未排下次 → 通知 leader |
| `cron-no-member-joined` | 每天  | 建立公會後 N 天無成員加入 → 通知 leader |
| `cron-not-joined-guild` | 每天  | 註冊後 N 天未加入公會 → 通知使用者 |
| `cron-raw-recording-cleanup` | 每天  | YouTube + AI 都完成 ∩ 7 天後 → 通知 RecordingProcessingWorker 刪 raw |


---

## 14. 歷史記錄(M11)

### 14.1 列表與詳情

| Method | Path | Handler | 權限  | 備註  | MoSCoW |
|--------|------|---------|-----|-----|--------|
| GET    | `/v1/guilds/:id/activities?status=ended` | `GuildActivitiesHandler` | 🏰  | 同 §4.1,只是 query filter | M      |
| GET    | `/v1/activities/:id` | `ActivityGetHandler` | 🎯  | 同 §4.1 | M      |
| GET    | `/v1/activities/:id/review-record` | `ActivityReviewRecordHandler` | 🎯  | 復盤第二階段歸納 | M      |

### 14.2 錄影 URL

直接從 `Activity.youtube_url` 讀取,前端嵌 YouTube iframe player。**不需要簽名 URL endpoint**(YouTube unlisted 不靠 signed URL)。

> ⚠️ 隱私邊界提醒:Unlisted URL 一旦給出去,前成員 / 退會成員仍可能透過保存的 URL 觀看。詳見 follow-up 文件 §1。


---

## 15. 既有 Endpoint 處理表

對齊現有 jpcorrect-backend 路由,標明 Talkuma MVP 的處理方向:

| 路由  | 狀態  | 說明  |
|-----|-----|-----|
| `GET /healthz` | 沿用  |     |
| `GET /ws` | 擴充  | 新增 Talkuma 練習 / 復盤 / 動態牆 / 通知 channels(§8.2、§12.2、§5.2、§6.4) |
| `/v1/mark-accent`、`/v1/mark-furigana`、`/v1/dict-query`、`/v1/sentence-query`、`/v1/usage-query/*` | 沿用(API Tools Proxy) | 既有的日語處理工具,Talkuma 可能間接使用(例如逐字稿渲染 furigana) |
| `/v1/users/*`(既有) | 部分沿用 | 補充 §1 列出的新 endpoint |
| `/v1/practices/*`(既有) | **deprecate** | Talkuma MVP 改用 `/v1/activities/*`。後台 / debug 場景保留 |
| `/v1/mistakes/*`(既有) | 部分沿用 | 補充 §11.1 新增的 patch endpoint |
| `/v1/transcripts/*`(既有) | 部分沿用 | 仍可作為 admin / debug |
| `/v1/event-attendees/*`(既有) | 沿用為 admin | Talkuma MVP 前端不直接呼叫,attendee 由「進入練習房間」流程自動建立 |
| `/v1/guilds/*`(既有) | 部分沿用 | 補充 §3 列出的新 endpoint |


---

## 16. 給後端組的實作建議

### 16.1 Sprint 切分建議

#### 整體進度總覽

| Sprint | 主軸  | 包含區塊 | 前置依賴 | 解鎖什麼 |
|--------|-----|------|------|------|
| 1      | 基礎建設 + 認證 | §1 認證、Schema §1 既有表調整、Schema §2.1 Activity 表、§4 權限 helper 簽名 | —    | 解鎖所有後續 sprint(沒有這些 schema 跟權限 helper,其他 endpoint 開不下去) |
| 2      | 公會 + 排程 | §3 公會、§4 Activity/排程、§3.5 預設時段、§1.3 邀請接受 | Sprint 1 | 前端可以開始接公會層 UI(動態牆除外) |
| 3      | 動態牆 + 通知 | §5 動態牆、§6 通知、WebSocket Hub 擴充、NotificationDispatcher worker | Sprint 1、2 | 前端可以接動態牆 + 通知鈴鐺 |
| 4      | 練習房間 | §8 練習房間 endpoint + WebSocket message、WebRTC Hub 業務事件整合、§13 cron 基礎建設 | Sprint 1、2 | 內部測試端到端練習流程 |
| 5      | AI + 回饋 + 復盤 | §9 AI 分析、§10 Chatbot、§11 會後回饋、§12 復盤房間 | Sprint 4(需要練習產生資料) | 完整一輪活動可跑完 |
| 6      | 收尾 + 錄影 | §14 歷史記錄、RecordingProcessingWorker(YouTube)、§13.2 cron 補齊 | Sprint 5 | 可以正式內測 |

#### Sprint 1 — 基礎建設 + 認證

| 項目  | 對應  | 類型  | 備註  |
|-----|-----|-----|-----|
| User 表加 supabase_user_id 欄位 | Schema §1.4 | DB  |     |
| EventMode 拿掉 discussion | Schema §1.1 | DB  |     |
| Event 表加 activity_id / recording_started_by 等 | Schema §1.2 | DB  |     |
| Mistake 表擴充 confidence / latest_correction_\* / interacted_at | Schema §1.3 | DB  |     |
| Activity 表 | Schema §2.1 | DB  | 核心抽象,必須早 |
| 權限 helper function 簽名實作 | Schema §4 | Code | 簽名先確定,實作可粗糙 |
| `POST /v1/users/init` | API §1.1 | Endpoint | First-login 用 |
| `GET /v1/users/me`、`PUT /v1/users/me` | API §1.1 | Endpoint |     |

#### Sprint 2 — 公會 + 排程

| 項目  | 對應  | 類型  | 備註  |
|-----|-----|-----|-----|
| InviteLink 表 | Schema §2.2 | DB  |     |
| GuildDefaultSlot 表 | Schema §2.3 | DB  |     |
| Topic 表 + ReportThemeSuggestion 表 | Schema §2.4、§2.5 | DB  | 內容由平台後台填 |
| JoinRequest 表 | Schema §3.1 | DB  | Should |
| 公會 endpoint(剩下) | API §3.1–§3.4 | Endpoint | 部分既有 |
| 預設時段 endpoint | API §3.5 | Endpoint |     |
| 公會板 endpoint | API §3.6 | Endpoint | Should |
| Activity CRUD endpoint | API §4.1 | Endpoint | 含建立配對 Event 的 transaction |
| 報告輪值建議 endpoint | API §4.2 | Endpoint |     |
| 邀請流程(invite-info、accept) | API §1.3 | Endpoint |     |
| 題庫查詢 endpoint | API §7 | Endpoint |     |

#### Sprint 3 — 動態牆 + 通知

| 項目  | 對應  | 類型  | 備註  |
|-----|-----|-----|-----|
| FeedPost 表 + FeedComment 表 | Schema §2.10、§2.11 | DB  |     |
| Notification 表 + PushSubscription 表 | Schema §2.9、§3.2 | DB  |     |
| WebSocket Hub 擴充 — 業務 channel | API §0.4、§5.2、§6.4 | Code | 新增 `feed:{guild_id}`、`notifications:{user_id}` |
| 動態牆 endpoint | API §5 | Endpoint |     |
| 通知 endpoint | API §6 | Endpoint |     |
| NotificationDispatcher worker | API §0.4、§6.3 | Worker | 業務事件 → 三管道分派 |
| WebPushSender、EmailSender worker | API §0.4 | Worker | 串外部 service |
| Email service 選型 + 設定 | API §17.3 | 決策 + 整合 | Resend / Postmark |

#### Sprint 4 — 練習房間

| 項目  | 對應  | 類型  | 備註  |
|-----|-----|-----|-----|
| PracticeNote 表 | Schema §2.12 | DB  |     |
| WebSocket Hub 擴充 — 練習 channel | API §8.2 | Code | `practice:{activity_id}` + 新 message types |
| WebRTC Hub 業務事件整合 | API §8.3 | Code | Hub 內 call repository |
| 練習房間 endpoint | API §8.1、§8.4 | Endpoint | eligibility、個人筆記 |
| §13 cron 基礎建設 | API §13.2 | Worker | ticker / robfig/cron 起一個容器,先把 starting_soon、stale-activity 兩個跑起來 |

#### Sprint 5 — AI + 回饋 + 復盤

| 項目  | 對應  | 類型  | 備註  |
|-----|-----|-----|-----|
| FeedbackProgress 表 | Schema §2.6 | DB  |     |
| SelectedSentence 表 | Schema §2.7 | DB  |     |
| ReviewRecord 表 | Schema §2.8 | DB  |     |
| ChatbotUsage 表 | Schema §2.13 | DB  |     |
| AIAnalysisWorker | API §9.1 | Worker | MVP 接人工後台 |
| `/internal/ai-analysis/submit` 介面 | API §9.2 | Endpoint | shared secret 認證 |
| 簡易人工後台 UI | API §9.2 | UI  | 內部用,接 submit endpoint |
| LLM service 選型 + 串接 | API §17.2 | 決策 + 整合 | OpenAI / Anthropic |
| Chatbot — 會後回饋 endpoint | API §10.1 | Endpoint | SSE streaming |
| Chatbot — 復盤 endpoint | API §10.2 | Endpoint |     |
| Chatbot — 歷史記錄繼續對話 | API §10.3 | Endpoint |     |
| 會後回饋 endpoint | API §11.1–§11.3 | Endpoint | 取資料、選題、進度 |
| 降級機制 cron | API §11.4 | Worker |     |
| 復盤房間 endpoint + WebSocket | API §12 | Endpoint + WS |     |

#### Sprint 6 — 收尾 + 錄影

| 項目  | 對應  | 類型  | 備註  |
|-----|-----|-----|-----|
| 歷史記錄 endpoint | API §14 | Endpoint | 大部分已有,差 review-record |
| RecordingProcessingWorker | API §0.4 | Worker | 合成 podcast 風格 + YouTube 上傳 |
| YouTube Data API 串接 + quota 設定 | API §17 | 整合  |     |
| §13.2 剩下的 cron 補齊 | API §13.2 | Worker | invite-link、no-next-activity 等通知類 |
| `/v1/activities/:id/review-record` endpoint | API §14.1 | Endpoint |     |
| Web Push VAPID key 設定 | API §6.2 | 設定  |     |
| 整體 smoke test | —   | 測試  | Staff 內測前的 end-to-end 跑一次 |

> 📝 表格中「對應」欄位的「Schema §x」指 Schema 補齊 Plan 的章節,「API §x」指本文件的章節。

### 16.2 與 WebRTC 後端的整合

WebRTC 部分既有的 `internal/api/webrtc.go` Hub pattern + signaling 已經跑得起來。Talkuma 的擴充:


1. **新增業務 channel**:`practice:{activity_id}`、`review:{activity_id}`、`feed:{guild_id}`、`notifications:{user_id}`
2. **新增 message type**:見 §8.2、§12.2、§5.2、§6.4
3. **業務事件直接 call repository**(同 process)
4. **Rate Limiter 沿用**

### 16.3 與正式版 AI 介面對齊

§9.2 內部 endpoint(`/internal/ai-analysis/submit`)的 request/response schema 需要在 day 1 鎖死,人工後台與未來自動 AI service 都實作同樣介面。建議:

* 介面文件以 OpenAPI / JSON schema 形式存 repo
* 人工後台用 backend 內部一個簡單的 admin UI 接這個 endpoint


---

## 17. 跨域議題與待釐清

### 17.1 Schema Plan §7.4 還沒定案

User 表的 supabase_user_id 欄位設計。後端組需確認後再開始 Sprint 1。

### 17.2 LLM service 選型

§10 Chatbot 需要 OpenAI / Anthropic / 其他 LLM provider。MVP 階段:

* Provider 選擇
* API key 管理
* Streaming 實作(SSE)
* 系統 prompt 設計(對應 content 團隊的 chatbot 角色定位文件)

### 17.3 Email service 選型

NotificationDispatcher 的 Email 管道。建議 Resend(API 簡潔)或 Postmark。需要:

* 域名 SPF / DKIM 設定
* Transactional email template 系統(MVP 可硬編碼)

### 17.4 LLM session state 儲存

§10.1 內存 map 還是 Redis?MVP 階段建議內存 map + sync.RWMutex(簡單),session timeout 30 分鐘。流量大時再上 Redis。

### 17.5 WebRTC TURN server

既有架構未提到 TURN。Production 環境跨網路 NAT 場景需要 TURN。需要:

* 自架 coturn 還是用第三方(例如 Twilio Network Traversal Service)
* 與 WebRTC 後端團隊確認


---

## 18. 後續文件

完成本文件對齊後,下一輪會產出:

* **資料模型詳細 schema**(欄位型別、SQL 等)由後端組實作 GORM model 後反饋
* **OpenAPI spec**(自動生成或手寫)供前端組生成 type-safe client
* **WebSocket message protocol 完整 spec**(JSON schema)
