package repository

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

func TestGetByGuildAndUserExcludesLeftMember(t *testing.T) {
	db := testutil.NewPostgresDB(t)

	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { assert.NoError(t, tx.Rollback().Error) })

	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "former guild member"}
	guild := domain.Guild{ID: uuid.New(), Name: "membership authorization test"}
	attendee := domain.GuildAttendee{ID: uuid.New(), GuildID: guild.ID, UserID: user.ID, Role: domain.GuildAttendeeRoleMaster}
	require.NoError(t, tx.Create(&user).Error)
	require.NoError(t, tx.Create(&guild).Error)
	require.NoError(t, tx.Create(&attendee).Error)

	repo := NewGormGuildAttendeeRepository(tx)
	active, err := repo.GetByGuildAndUser(context.Background(), guild.ID, user.ID)
	require.NoError(t, err)
	require.Equal(t, attendee.ID, active.ID)

	leftAt := time.Now()
	require.NoError(t, tx.Model(&attendee).Update("left_at", leftAt).Error)
	former, err := repo.GetByGuildAndUser(context.Background(), guild.ID, user.ID)
	require.Nil(t, former)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCreateInviteLinkWithTxConcurrentRotation(t *testing.T) {
	db := testutil.NewPostgresDB(t)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	guild := domain.Guild{ID: uuid.New(), Name: "concurrent invite rotation test"}
	require.NoError(t, db.Create(&guild).Error)
	creator := domain.User{ID: uuid.New(), Email: "invite-rotation@example.com", Name: "invite creator"}
	require.NoError(t, db.Create(&creator).Error)

	repo := NewGormGuildRepository(db)
	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)

	for range 2 {
		go func() {
			now := time.Now()
			invite := &domain.InviteLink{
				ID:              uuid.New(),
				GuildID:         guild.ID,
				Token:           strings.ReplaceAll(uuid.NewString(), "-", ""),
				ExpiresAt:       now.Add(7 * 24 * time.Hour),
				CreatedByUserID: creator.ID,
			}
			ready.Done()
			<-start
			results <- repo.CreateInviteLinkWithTx(context.Background(), guild.ID, invite, now)
		}()
	}

	ready.Wait()
	close(start)
	for range 2 {
		require.NoError(t, <-results)
	}

	var total, active int64
	require.NoError(t, db.Model(&domain.InviteLink{}).Where("guild_id = ?", guild.ID).Count(&total).Error)
	require.NoError(t, db.Model(&domain.InviteLink{}).
		Where("guild_id = ? AND expires_at > ?", guild.ID, time.Now()).
		Count(&active).Error)
	assert.EqualValues(t, 2, total)
	assert.EqualValues(t, 1, active)
}
