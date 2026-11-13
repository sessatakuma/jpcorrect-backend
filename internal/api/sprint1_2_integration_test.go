package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/database"
	"jpcorrect-backend/internal/domain"
)

// newIntegrationAPI connects to DATABASE_URL and migrates every model the
// handlers touch. The test is skipped when no PostgreSQL instance is available.
func newIntegrationAPI(t *testing.T) *API {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set, skipping PostgreSQL integration test")
	}
	db, err := database.NewGormDB(databaseURL)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&domain.User{},
		&domain.Guild{},
		&domain.GuildAttendee{},
		&domain.GuildDefaultSlot{},
		&domain.JoinRequest{},
		&domain.Topic{},
		&domain.ReportThemeSuggestion{},
		&domain.Activity{},
		&domain.Event{},
		&domain.EventAttendee{},
		&domain.Transcript{},
		&domain.Mistake{},
		&domain.GuildInvite{},
	))
	return NewAPI("", "", &http.Transport{}, db, "", nil)
}

func callHandler(t *testing.T, handler gin.HandlerFunc, method, target string, body []byte, params gin.Params, caller *uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	c.Request = httptest.NewRequest(method, target, reader)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	if caller != nil {
		c.Set("userID", *caller)
	}
	handler(c)
	return w
}

func TestActivityCreateGeneratesIDs(t *testing.T) {
	api := newIntegrationAPI(t)
	ctx := t.Context()

	master := &domain.User{Email: uuid.NewString() + "@example.com", Name: "activity master"}
	require.NoError(t, api.userRepo.Create(ctx, master))
	guild := &domain.Guild{Name: "activity guild"}
	require.NoError(t, api.guildRepo.Create(ctx, guild))
	attendee := &domain.GuildAttendee{
		GuildID: guild.ID,
		UserID:  master.ID,
		Role:    domain.GuildAttendeeRoleMaster,
	}
	require.NoError(t, api.guildAttendeeRepo.Create(ctx, attendee))

	t.Cleanup(func() {
		api.db.WithContext(ctx).Unscoped().Where("activity_id IN (?)",
			api.db.Model(&domain.Activity{}).Select("id").Where("guild_id = ?", guild.ID),
		).Delete(&domain.Event{})
		api.db.WithContext(ctx).Unscoped().Where("guild_id = ?", guild.ID).Delete(&domain.Activity{})
		api.db.WithContext(ctx).Unscoped().Where("guild_id = ?", guild.ID).Delete(&domain.GuildAttendee{})
		api.db.WithContext(ctx).Unscoped().Delete(&domain.Guild{}, "id = ?", guild.ID)
		api.db.WithContext(ctx).Unscoped().Delete(&domain.User{}, "id = ?", master.ID)
	})

	practiceAt := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(map[string]any{
		"practice_at": practiceAt,
		"mode":        "report",
	})
	require.NoError(t, err)

	w := callHandler(t, api.ActivityCreateHandler, http.MethodPost,
		"/v1/guilds/"+guild.ID.String()+"/activities", payload,
		gin.Params{{Key: "id", Value: guild.ID.String()}}, &master.ID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp activityDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Activity)
	require.Len(t, resp.Events, 2)

	assert.NotEqual(t, uuid.Nil, resp.Activity.ID, "activity must have a generated UUID")
	assert.NotEqual(t, uuid.Nil, resp.Events[0].ID, "practice event must have a generated UUID")
	assert.NotEqual(t, uuid.Nil, resp.Events[1].ID, "review event must have a generated UUID")
	assert.NotEqual(t, resp.Events[0].ID, resp.Events[1].ID, "events need distinct primary keys")

	var stored domain.Activity
	require.NoError(t, api.db.WithContext(ctx).First(&stored, "id = ?", resp.Activity.ID).Error)
	var storedEvents []domain.Event
	require.NoError(t, api.db.WithContext(ctx).Where("activity_id = ?", stored.ID).Find(&storedEvents).Error)
	assert.Len(t, storedEvents, 2)
}

func TestGuildApplicationApproveRejectsCrossGuildAndRepeat(t *testing.T) {
	api := newIntegrationAPI(t)
	ctx := t.Context()

	masterA := &domain.User{Email: uuid.NewString() + "@example.com", Name: "master a"}
	masterB := &domain.User{Email: uuid.NewString() + "@example.com", Name: "master b"}
	applicant := &domain.User{Email: uuid.NewString() + "@example.com", Name: "applicant"}
	require.NoError(t, api.userRepo.Create(ctx, masterA))
	require.NoError(t, api.userRepo.Create(ctx, masterB))
	require.NoError(t, api.userRepo.Create(ctx, applicant))

	guildA := &domain.Guild{Name: "guild a"}
	guildB := &domain.Guild{Name: "guild b"}
	require.NoError(t, api.guildRepo.Create(ctx, guildA))
	require.NoError(t, api.guildRepo.Create(ctx, guildB))

	require.NoError(t, api.guildAttendeeRepo.Create(ctx, &domain.GuildAttendee{
		GuildID: guildA.ID, UserID: masterA.ID, Role: domain.GuildAttendeeRoleMaster,
	}))
	require.NoError(t, api.guildAttendeeRepo.Create(ctx, &domain.GuildAttendee{
		GuildID: guildB.ID, UserID: masterB.ID, Role: domain.GuildAttendeeRoleMaster,
	}))

	t.Cleanup(func() {
		api.db.WithContext(ctx).Unscoped().Where("guild_id IN ?", []uuid.UUID{guildA.ID, guildB.ID}).Delete(&domain.JoinRequest{})
		api.db.WithContext(ctx).Unscoped().Where("guild_id IN ?", []uuid.UUID{guildA.ID, guildB.ID}).Delete(&domain.GuildAttendee{})
		api.db.WithContext(ctx).Unscoped().Delete(&domain.Guild{}, "id IN ?", []uuid.UUID{guildA.ID, guildB.ID})
		api.db.WithContext(ctx).Unscoped().Delete(&domain.User{}, "id IN ?", []uuid.UUID{masterA.ID, masterB.ID, applicant.ID})
	})

	requestToB := &domain.JoinRequest{GuildID: guildB.ID, UserID: applicant.ID, Status: domain.JoinRequestStatusPending}
	require.NoError(t, api.joinRequestRepo.Create(ctx, requestToB))

	// Guild A's master must not be able to approve guild B's request.
	w := callHandler(t, api.GuildApplicationApproveHandler, http.MethodPost,
		"/v1/guilds/"+guildA.ID.String()+"/applications/"+requestToB.ID.String()+"/approve", nil,
		gin.Params{{Key: "id", Value: guildA.ID.String()}, {Key: "app_id", Value: requestToB.ID.String()}}, &masterA.ID)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	stored, err := api.joinRequestRepo.GetByID(ctx, requestToB.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.JoinRequestStatusPending, stored.Status, "cross-guild approval must not change the request")

	// The owning guild can approve it once.
	requestToA := &domain.JoinRequest{GuildID: guildA.ID, UserID: applicant.ID, Status: domain.JoinRequestStatusPending}
	require.NoError(t, api.joinRequestRepo.Create(ctx, requestToA))
	w = callHandler(t, api.GuildApplicationApproveHandler, http.MethodPost,
		"/v1/guilds/"+guildA.ID.String()+"/applications/"+requestToA.ID.String()+"/approve", nil,
		gin.Params{{Key: "id", Value: guildA.ID.String()}, {Key: "app_id", Value: requestToA.ID.String()}}, &masterA.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	_, err = api.guildAttendeeRepo.GetByGuildAndUser(ctx, guildA.ID, applicant.ID)
	assert.NoError(t, err, "approved applicant becomes a member")

	// A second approval of the same request is rejected.
	w = callHandler(t, api.GuildApplicationApproveHandler, http.MethodPost,
		"/v1/guilds/"+guildA.ID.String()+"/applications/"+requestToA.ID.String()+"/approve", nil,
		gin.Params{{Key: "id", Value: guildA.ID.String()}, {Key: "app_id", Value: requestToA.ID.String()}}, &masterA.ID)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}
