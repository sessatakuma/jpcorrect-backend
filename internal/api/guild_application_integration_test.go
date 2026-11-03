package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/database"
	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"
)

func TestGuildApplicationFirstSubmissionWithRepository(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set, skipping PostgreSQL integration test")
	}

	db, err := database.NewGormDB(databaseURL)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	ctx := context.Background()
	require.NoError(t, db.WithContext(ctx).AutoMigrate(
		&domain.User{}, &domain.Guild{}, &domain.GuildAttendee{}, &domain.GuildApplication{},
	))

	tx := db.WithContext(ctx).Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { require.NoError(t, tx.Rollback().Error) })
	guild := domain.Guild{ID: uuid.New(), Name: "first guild application test"}
	require.NoError(t, tx.WithContext(ctx).Create(&guild).Error)
	callerID := uuid.New()
	applicationRepo := repository.NewGuildApplicationRepository(tx)
	app, err := applicationRepo.GetPendingByGuildAndUser(ctx, guild.ID, callerID)
	require.Nil(t, app)
	require.ErrorIs(t, err, domain.ErrNotFound)

	api := &API{
		guildRepo:         repository.NewGormGuildRepository(tx),
		guildAttendeeRepo: repository.NewGormGuildAttendeeRepository(tx),
		applicationRepo:   applicationRepo,
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/guilds/:id/applications", func(c *gin.Context) {
		c.Set("userID", callerID)
	}, api.GuildApplicationCreateHandler)
	path := "/v1/guilds/" + guild.ID.String() + "/applications"

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, path, nil))
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	var created GuildApplicationResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &created))
	require.Equal(t, guild.ID, created.GuildID)
	require.Equal(t, callerID, created.UserID)
	require.Equal(t, domain.GuildApplicationStatusPending, created.Status)

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, path, nil))
	require.Equal(t, http.StatusBadRequest, second.Code, second.Body.String())

	var count int64
	require.NoError(t, tx.WithContext(ctx).Model(&domain.GuildApplication{}).
		Where("guild_id = ? AND user_id = ? AND status = ?", guild.ID, callerID, domain.GuildApplicationStatusPending).
		Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestDeletedGuildCannotReviewApplications(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set, skipping PostgreSQL integration test")
	}
	db, err := database.NewGormDB(databaseURL)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&domain.User{}, &domain.Guild{}, &domain.GuildAttendee{}, &domain.GuildApplication{}))
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { require.NoError(t, tx.Rollback().Error) })
	guild := domain.Guild{ID: uuid.New(), Name: "deleted guild"}
	otherGuild := domain.Guild{ID: uuid.New(), Name: "active guild"}
	masterID, applicantID := uuid.New(), uuid.New()
	require.NoError(t, tx.Create(&guild).Error)
	require.NoError(t, tx.Create(&otherGuild).Error)
	require.NoError(t, tx.Create(&domain.GuildAttendee{ID: uuid.New(), GuildID: guild.ID, UserID: masterID, Role: domain.GuildAttendeeRoleMaster}).Error)
	apps := []domain.GuildApplication{
		{ID: uuid.New(), GuildID: guild.ID, UserID: applicantID, Status: domain.GuildApplicationStatusPending},
		{ID: uuid.New(), GuildID: otherGuild.ID, UserID: applicantID, Status: domain.GuildApplicationStatusPending},
	}
	require.NoError(t, tx.Create(&apps).Error)
	require.NoError(t, tx.Delete(&guild).Error)
	api := &API{guildRepo: repository.NewGormGuildRepository(tx), guildAttendeeRepo: repository.NewGormGuildAttendeeRepository(tx), applicationRepo: repository.NewGuildApplicationRepository(tx)}
	for _, handler := range []gin.HandlerFunc{api.GuildApplicationApproveHandler, api.GuildApplicationRejectHandler} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		c.Params = gin.Params{{Key: "id", Value: guild.ID.String()}, {Key: "app_id", Value: apps[0].ID.String()}}
		c.Set("userID", masterID)
		handler(c)
		require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	}
	for _, app := range apps {
		stored, err := api.applicationRepo.GetByID(context.Background(), app.ID)
		require.NoError(t, err)
		require.Equal(t, domain.GuildApplicationStatusPending, stored.Status)
	}
	var count int64
	require.NoError(t, tx.Model(&domain.GuildAttendee{}).Where("user_id = ?", applicantID).Count(&count).Error)
	require.Zero(t, count)
}
