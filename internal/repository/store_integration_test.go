package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// countRows counts the rows of model matching query, ignoring soft-deleted
// rows, so an assertion of 0 means "not visible".
func countRows(t *testing.T, db *gorm.DB, model any, query string, args ...any) int64 {
	t.Helper()

	var count int64
	require.NoError(t, db.Model(model).Where(query, args...).Count(&count).Error)
	return count
}

// TestStoreTxRollsBackWritesFromEveryRepository proves that one failed Tx
// discards the writes of every repository in the RepoSet, which also shows that
// all of them are bound to the same transaction.
func TestStoreTxRollsBackWritesFromEveryRepository(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	store := NewStore(db)
	ctx := context.Background()

	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "tx rollback user"}
	guild := domain.Guild{ID: uuid.New(), Name: "tx rollback guild"}
	attendee := domain.GuildAttendee{ID: uuid.New(), UserID: user.ID}
	event := domain.Event{ID: uuid.New(), Title: "tx rollback event", StartTime: time.Now()}
	eventAttendee := domain.EventAttendee{ID: uuid.New(), EventID: event.ID, UserID: user.ID}
	transcript := domain.Transcript{ID: uuid.New(), EventID: event.ID, UserID: user.ID, Content: "rollback me"}
	mistake := domain.Mistake{ID: uuid.New(), EventID: event.ID, UserID: user.ID, OriginText: "その", FixedText: "それ"}
	boom := errors.New("fail after writing through every repository")

	err := store.Tx(ctx, func(r RepoSet) error {
		require.NoError(t, r.UserRepo.Create(ctx, &user))
		require.NoError(t, r.GuildRepo.Create(ctx, &guild))
		attendee.GuildID = guild.ID
		require.NoError(t, r.GuildAttendeeRepo.Create(ctx, &attendee))
		require.NoError(t, r.EventRepo.Create(ctx, &event))
		require.NoError(t, r.EventAttendeeRepo.Create(ctx, &eventAttendee))
		require.NoError(t, r.TranscriptRepo.Create(ctx, &transcript))
		require.NoError(t, r.MistakeRepo.Create(ctx, &mistake))
		return boom
	})
	require.ErrorIs(t, err, boom)

	for _, check := range []struct {
		name  string
		model any
		id    uuid.UUID
	}{
		{"UserRepo", &domain.User{}, user.ID},
		{"GuildRepo", &domain.Guild{}, guild.ID},
		{"GuildAttendeeRepo", &domain.GuildAttendee{}, attendee.ID},
		{"EventRepo", &domain.Event{}, event.ID},
		{"EventAttendeeRepo", &domain.EventAttendee{}, eventAttendee.ID},
		{"TranscriptRepo", &domain.Transcript{}, transcript.ID},
		{"MistakeRepo", &domain.Mistake{}, mistake.ID},
	} {
		assert.Zero(t, countRows(t, db, check.model, "id = ?", check.id),
			"%s write survived the rollback", check.name)
	}
}

// TestStoreTxRepanicsAfterRollback proves that a panic rolls the writes back and
// still propagates to the caller.
func TestStoreTxRepanicsAfterRollback(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	store := NewStore(db)
	ctx := context.Background()

	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "tx panic user"}
	guild := domain.Guild{ID: uuid.New(), Name: "tx panic guild"}

	require.PanicsWithValue(t, "boom", func() {
		_ = store.Tx(ctx, func(r RepoSet) error {
			require.NoError(t, r.UserRepo.Create(ctx, &user))
			require.NoError(t, r.GuildRepo.Create(ctx, &guild))
			panic("boom")
		})
	})

	assert.Zero(t, countRows(t, db, &domain.User{}, "id = ?", user.ID))
	assert.Zero(t, countRows(t, db, &domain.Guild{}, "id = ?", guild.ID))
}

// TestStoreTxCommitsOnSuccess proves that a nil callback commits: the writes are
// invisible to other connections while the transaction is open and visible to
// repositories built on the root handle afterwards.
func TestStoreTxCommitsOnSuccess(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	store := NewStore(db)
	ctx := context.Background()

	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "tx commit user"}
	guild := domain.Guild{ID: uuid.New(), Name: "tx commit guild"}

	require.NoError(t, store.Tx(ctx, func(r RepoSet) error {
		if err := r.UserRepo.Create(ctx, &user); err != nil {
			return err
		}
		if err := r.GuildRepo.Create(ctx, &guild); err != nil {
			return err
		}

		// Still uncommitted: another connection cannot see the rows yet.
		assert.Zero(t, countRows(t, db, &domain.User{}, "id = ?", user.ID),
			"a row was visible outside the transaction before commit")
		return nil
	}))

	committedUser, err := NewGormUserRepository(db).GetByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.Email, committedUser.Email)

	committedGuild, err := NewGormGuildRepository(db).GetByID(ctx, guild.ID)
	require.NoError(t, err)
	assert.Equal(t, guild.Name, committedGuild.Name)
}

// TestStoreTxRollsBackRepositoryInternalTransactions proves that the
// transactions a repository opens internally become savepoints of the outer
// transaction instead of independent transactions: their writes are already
// visible inside the callback and disappear when the outer transaction fails.
func TestStoreTxRollsBackRepositoryInternalTransactions(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateWithMaster", func(t *testing.T) {
		db := testutil.NewPostgresDB(t)
		store := NewStore(db)
		boom := errors.New("fail the outer transaction")

		user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "nested create master"}
		require.NoError(t, db.Create(&user).Error)
		guild := domain.Guild{ID: uuid.New(), Name: "nested create guild"}
		attendee := domain.GuildAttendee{ID: uuid.New(), UserID: user.ID}

		err := store.Tx(ctx, func(r RepoSet) error {
			require.NoError(t, r.GuildRepo.CreateWithMaster(ctx, &guild, &attendee))

			// The nested transaction joined the outer one: its writes are
			// already readable through the same transaction.
			nested, err := r.GuildAttendeeRepo.GetByGuildAndUser(ctx, guild.ID, user.ID)
			require.NoError(t, err)
			require.Equal(t, domain.GuildAttendeeRoleMaster, nested.Role)

			return boom
		})
		require.ErrorIs(t, err, boom)

		assert.Zero(t, countRows(t, db, &domain.Guild{}, "id = ?", guild.ID))
		assert.Zero(t, countRows(t, db, &domain.GuildAttendee{}, "id = ?", attendee.ID))
	})

	t.Run("TransferLeader", func(t *testing.T) {
		db := testutil.NewPostgresDB(t)
		store := NewStore(db)
		boom := errors.New("fail the outer transaction")

		master := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "nested transfer master"}
		member := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "nested transfer member"}
		require.NoError(t, db.Create(&[]domain.User{master, member}).Error)
		guild := domain.Guild{ID: uuid.New(), Name: "nested transfer guild"}
		require.NoError(t, db.Create(&guild).Error)
		attendees := []domain.GuildAttendee{
			{ID: uuid.New(), GuildID: guild.ID, UserID: master.ID, Role: domain.GuildAttendeeRoleMaster},
			{ID: uuid.New(), GuildID: guild.ID, UserID: member.ID, Role: domain.GuildAttendeeRoleMember},
		}
		require.NoError(t, db.Create(&attendees).Error)

		err := store.Tx(ctx, func(r RepoSet) error {
			require.NoError(t, r.GuildRepo.TransferLeader(ctx, guild.ID, master.ID, member.ID))

			// Inside the transaction the roles are already swapped.
			swapped, err := r.GuildAttendeeRepo.GetByGuildAndUser(ctx, guild.ID, member.ID)
			require.NoError(t, err)
			require.Equal(t, domain.GuildAttendeeRoleMaster, swapped.Role)

			return boom
		})
		require.ErrorIs(t, err, boom)

		rootRepo := NewGormGuildAttendeeRepository(db)
		formerMaster, err := rootRepo.GetByGuildAndUser(ctx, guild.ID, master.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.GuildAttendeeRoleMaster, formerMaster.Role)
		stillMember, err := rootRepo.GetByGuildAndUser(ctx, guild.ID, member.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.GuildAttendeeRoleMember, stillMember.Role)
	})

	t.Run("CreateInviteLinkWithTx", func(t *testing.T) {
		db := testutil.NewPostgresDB(t)
		store := NewStore(db)
		boom := errors.New("fail the outer transaction")

		creator := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "nested invite creator"}
		require.NoError(t, db.Create(&creator).Error)
		guild := domain.Guild{ID: uuid.New(), Name: "nested invite guild"}
		require.NoError(t, db.Create(&guild).Error)
		now := time.Now()
		invite := domain.InviteLink{
			ID:              uuid.New(),
			GuildID:         guild.ID,
			Token:           strings.ReplaceAll(uuid.NewString(), "-", ""),
			ExpiresAt:       now.Add(7 * 24 * time.Hour),
			CreatedByUserID: creator.ID,
		}

		err := store.Tx(ctx, func(r RepoSet) error {
			require.NoError(t, r.GuildRepo.CreateInviteLinkWithTx(ctx, guild.ID, &invite, now))

			// Inside the transaction the new link is already readable.
			active, err := r.GuildRepo.GetActiveInviteByGuildID(ctx, guild.ID, now)
			require.NoError(t, err)
			require.NotNil(t, active)
			require.Equal(t, invite.ID, active.ID)

			return boom
		})
		require.ErrorIs(t, err, boom)

		assert.Zero(t, countRows(t, db, &domain.InviteLink{}, "id = ?", invite.ID))
	})

	t.Run("Delete", func(t *testing.T) {
		db := testutil.NewPostgresDB(t)
		store := NewStore(db)
		boom := errors.New("fail the outer transaction")

		guild := domain.Guild{ID: uuid.New(), Name: "nested delete guild"}
		require.NoError(t, db.Create(&guild).Error)

		err := store.Tx(ctx, func(r RepoSet) error {
			require.NoError(t, r.GuildRepo.Delete(ctx, guild.ID))

			// Inside the transaction the guild is already soft-deleted.
			_, err := r.GuildRepo.GetByID(ctx, guild.ID)
			require.ErrorIs(t, err, domain.ErrNotFound)

			return boom
		})
		require.ErrorIs(t, err, boom)

		survivor, err := NewGormGuildRepository(db).GetByID(ctx, guild.ID)
		require.NoError(t, err)
		assert.Equal(t, guild.ID, survivor.ID)
	})
}

// TestStoreTxRollsBackOnCancelledContext proves that a request aborted after its
// writes but before commit persists nothing and reports the context error.
func TestStoreTxRollsBackOnCancelledContext(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	store := NewStore(db)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "tx cancelled user"}
	guild := domain.Guild{ID: uuid.New(), Name: "tx cancelled guild"}

	err := store.Tx(ctx, func(r RepoSet) error {
		require.NoError(t, r.UserRepo.Create(ctx, &user))
		require.NoError(t, r.GuildRepo.Create(ctx, &guild))
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)

	assert.Zero(t, countRows(t, db, &domain.User{}, "id = ?", user.ID))
	assert.Zero(t, countRows(t, db, &domain.Guild{}, "id = ?", guild.ID))
}

// TestStoreTxReturnsDomainErrorUnchanged proves that an error a repository
// already mapped to a domain sentinel reaches the caller as the same sentinel,
// with no wrapping, while the rest of the transaction is rolled back.
func TestStoreTxReturnsDomainErrorUnchanged(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	store := NewStore(db)
	ctx := context.Background()

	existing := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "tx duplicate owner"}
	require.NoError(t, db.Create(&existing).Error)
	guild := domain.Guild{ID: uuid.New(), Name: "tx duplicate guild"}
	duplicate := domain.User{ID: uuid.New(), Email: existing.Email, Name: "tx duplicate attempt"}

	err := store.Tx(ctx, func(r RepoSet) error {
		require.NoError(t, r.GuildRepo.Create(ctx, &guild))
		// Unique violation, mapped to domain.ErrDuplicateEntry by the repository.
		return r.UserRepo.Create(ctx, &duplicate)
	})
	require.ErrorIs(t, err, domain.ErrDuplicateEntry)
	require.EqualError(t, err, domain.ErrDuplicateEntry.Error())

	// The write before the failure is gone; the pre-existing row is untouched.
	assert.Zero(t, countRows(t, db, &domain.Guild{}, "id = ?", guild.ID))
	assert.EqualValues(t, 1, countRows(t, db, &domain.User{}, "id = ?", existing.ID))
}
