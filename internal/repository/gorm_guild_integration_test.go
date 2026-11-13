package repository

import (
	"context"
	"errors"
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

func TestCreateWithMasterConcurrentLimit(t *testing.T) {
	db := testutil.NewPostgresDB(t)

	// Keep the production pool small so the concurrent CreateWithMaster calls
	// really do contend on separate connections instead of queueing.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	user := domain.User{
		ID:    uuid.New(),
		Email: uuid.NewString() + "@example.com",
		Name:  "concurrent guild master test",
	}
	require.NoError(t, db.Create(&user).Error)

	guilds := []domain.Guild{
		{ID: uuid.New(), Name: "concurrent guild one"},
		{ID: uuid.New(), Name: "concurrent guild two"},
	}
	attendees := []domain.GuildAttendee{
		{ID: uuid.New(), UserID: user.ID},
		{ID: uuid.New(), UserID: user.ID},
	}

	repo := NewGormGuildRepository(db)
	start := make(chan struct{})
	results := make(chan error, len(guilds))
	var ready sync.WaitGroup
	ready.Add(len(guilds))

	for i := range guilds {
		go func(i int) {
			ready.Done()
			<-start
			results <- repo.CreateWithMaster(
				context.Background(),
				&guilds[i],
				&attendees[i],
			)
		}(i)
	}

	ready.Wait()
	close(start)

	var succeeded, limitReached int
	for range guilds {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrGuildLimitReached):
			limitReached++
		default:
			t.Fatalf("unexpected CreateWithMaster error: %v", err)
		}
	}

	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 1, limitReached)

	var activeMasterCount int64
	require.NoError(t, db.Model(&domain.GuildAttendee{}).
		Where("user_id = ? AND role = ? AND left_at IS NULL", user.ID, domain.GuildAttendeeRoleMaster).
		Count(&activeMasterCount).Error)
	assert.EqualValues(t, 1, activeMasterCount)

	var guildCount int64
	require.NoError(t, db.Model(&domain.Guild{}).
		Where("id IN ?", []uuid.UUID{guilds[0].ID, guilds[1].ID}).
		Count(&guildCount).Error)
	assert.EqualValues(t, 1, guildCount)
}

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

// TestTransferLeaderRejectsLeaderOfAnotherGuild pins the locked leader quota on
// the transfer path: the new leader may lead at most one guild (§3.3.1), so a
// target that already leads a live guild keeps both roles unchanged.
func TestTransferLeaderRejectsLeaderOfAnotherGuild(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	firstLeader := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "first leader"}
	memberLeader := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "member leader"}
	require.NoError(t, db.Create(&[]domain.User{firstLeader, memberLeader}).Error)

	repo := NewGormGuildRepository(db)

	firstGuild := domain.Guild{ID: uuid.New(), Name: "first guild"}
	require.NoError(t, repo.CreateWithMaster(ctx, &firstGuild, &domain.GuildAttendee{
		ID: uuid.New(), UserID: firstLeader.ID,
	}))

	// The target already leads a second guild.
	secondGuild := domain.Guild{ID: uuid.New(), Name: "second guild"}
	require.NoError(t, repo.CreateWithMaster(ctx, &secondGuild, &domain.GuildAttendee{
		ID: uuid.New(), UserID: memberLeader.ID,
	}))

	attendeeRepo := NewGormGuildAttendeeRepository(db)
	require.NoError(t, attendeeRepo.Create(ctx, &domain.GuildAttendee{
		ID: uuid.New(), GuildID: firstGuild.ID, UserID: memberLeader.ID,
		Role: domain.GuildAttendeeRoleMember,
	}))

	err := repo.TransferLeader(ctx, firstGuild.ID, firstLeader.ID, memberLeader.ID)
	require.ErrorIs(t, err, domain.ErrGuildLimitReached)

	first, err := attendeeRepo.GetByGuildAndUser(ctx, firstGuild.ID, firstLeader.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.GuildAttendeeRoleMaster, first.Role, "the caller must stay master")

	target, err := attendeeRepo.GetByGuildAndUser(ctx, firstGuild.ID, memberLeader.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.GuildAttendeeRoleMember, target.Role, "the target must stay a member")
}

// TestGuildAttendeeWritesEnforceLeaderQuota covers the generic attendee writes:
// promoting a row to an active master is subject to the same quota as guild
// creation and leader transfer.
func TestGuildAttendeeWritesEnforceLeaderQuota(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	leader := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "attendee quota leader"}
	require.NoError(t, db.Create(&leader).Error)

	guildRepo := NewGormGuildRepository(db)
	ledGuild := domain.Guild{ID: uuid.New(), Name: "led guild"}
	require.NoError(t, guildRepo.CreateWithMaster(ctx, &ledGuild, &domain.GuildAttendee{
		ID: uuid.New(), UserID: leader.ID,
	}))

	otherGuild := domain.Guild{ID: uuid.New(), Name: "other guild"}
	require.NoError(t, db.Create(&otherGuild).Error)

	repo := NewGormGuildAttendeeRepository(db)

	err := repo.Create(ctx, &domain.GuildAttendee{
		ID: uuid.New(), GuildID: otherGuild.ID, UserID: leader.ID,
		Role: domain.GuildAttendeeRoleMaster,
	})
	require.ErrorIs(t, err, domain.ErrGuildLimitReached, "a second active master row must be rejected")

	// A member row is always allowed, and updating it to master is not.
	member := domain.GuildAttendee{
		ID: uuid.New(), GuildID: otherGuild.ID, UserID: leader.ID,
		Role: domain.GuildAttendeeRoleMember,
	}
	require.NoError(t, repo.Create(ctx, &member))

	member.Role = domain.GuildAttendeeRoleMaster
	require.ErrorIs(t, repo.Update(ctx, &member), domain.ErrGuildLimitReached,
		"promoting a member to master must be rejected")

	stored, err := repo.GetByID(ctx, member.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.GuildAttendeeRoleMember, stored.Role, "the rejected update must not write")
}
