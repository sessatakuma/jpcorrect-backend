package repository

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

func TestGormGuildRepository_GetByID(t *testing.T) {
	guildID := uuid.New()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr error
	}{
		{
			name: "Success",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild" WHERE id = $1 AND "guild"."deleted_at" IS NULL ORDER BY "guild"."id" LIMIT $2`)).
					WithArgs(guildID, 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "level"}).
						AddRow(guildID, "Test Guild", "A guild for testing", 0))
			},
		},
		{
			name: "NotFound",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild" WHERE id = $1 AND "guild"."deleted_at" IS NULL ORDER BY "guild"."id" LIMIT $2`)).
					WithArgs(guildID, 1).
					WillReturnError(gorm.ErrRecordNotFound)
			},
			wantErr: domain.ErrNotFound,
		},
		{
			name: "DBError",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild" WHERE id = $1 AND "guild"."deleted_at" IS NULL ORDER BY "guild"."id" LIMIT $2`)).
					WithArgs(guildID, 1).
					WillReturnError(fmt.Errorf("db error"))
			},
			wantErr: fmt.Errorf("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			repo := NewGormGuildRepository(db)
			tt.mock(mock)

			guild, err := repo.GetByID(context.Background(), guildID)

			if tt.wantErr != nil {
				assert.Error(t, err)
				if tt.wantErr == domain.ErrNotFound {
					assert.ErrorIs(t, err, domain.ErrNotFound)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, guildID, guild.ID)
				assert.Equal(t, "Test Guild", guild.Name)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGormGuildRepository_Create(t *testing.T) {
	tests := []struct {
		name    string
		guild   *domain.Guild
		mock    func(mock sqlmock.Sqlmock)
		wantErr error
	}{
		{
			name: "Success",
			guild: &domain.Guild{
				Name:        "New Guild",
				Description: "A new guild",
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild"`)).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
		},
		{
			name: "DuplicateEntry",
			guild: &domain.Guild{
				Name:        "Duplicate Guild",
				Description: "A guild with duplicate name",
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild"`)).
					WillReturnError(&pgconn.PgError{Code: "23505"})
				mock.ExpectRollback()
			},
			wantErr: domain.ErrDuplicateEntry,
		},
		{
			name: "DBError",
			guild: &domain.Guild{
				Name:        "DB Error Guild",
				Description: "A guild that errors",
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild"`)).
					WillReturnError(fmt.Errorf("db error"))
				mock.ExpectRollback()
			},
			wantErr: fmt.Errorf("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			repo := NewGormGuildRepository(db)
			tt.mock(mock)

			err := repo.Create(context.Background(), tt.guild)

			if tt.wantErr != nil {
				assert.Error(t, err)
				if tt.wantErr == domain.ErrDuplicateEntry {
					assert.ErrorIs(t, err, domain.ErrDuplicateEntry)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEqual(t, uuid.Nil, tt.guild.ID)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGormGuildRepository_Update(t *testing.T) {
	guildID := uuid.New()

	tests := []struct {
		name    string
		guild   *domain.Guild
		mock    func(mock sqlmock.Sqlmock)
		wantErr bool
	}{
		{
			name: "Success",
			guild: &domain.Guild{
				ID:          guildID,
				Name:        "Updated Guild",
				Description: "Updated description",
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE "guild"`)).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
		},
		{
			name: "DBError",
			guild: &domain.Guild{
				ID:          guildID,
				Name:        "DB Error Guild",
				Description: "DB error description",
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE "guild"`)).
					WillReturnError(fmt.Errorf("db error"))
				mock.ExpectRollback()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			repo := NewGormGuildRepository(db)
			tt.mock(mock)

			err := repo.Update(context.Background(), tt.guild)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGormGuildRepository_Delete(t *testing.T) {
	guildID := uuid.New()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr error
	}{
		{
			name: "Success",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "guild_attendee" WHERE guild_id = $1`)).
					WithArgs(guildID).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE "guild" SET "deleted_at"=$1 WHERE id = $2 AND "guild"."deleted_at" IS NULL`)).
					WithArgs(sqlmock.AnyArg(), guildID).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
		},
		{
			name: "HasRelatedRecords",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "guild_attendee" WHERE guild_id = $1`)).
					WithArgs(guildID).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectRollback()
			},
			wantErr: domain.ErrHasRelatedRecords,
		},
		{
			name: "DBError",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "guild_attendee" WHERE guild_id = $1`)).
					WithArgs(guildID).
					WillReturnError(fmt.Errorf("db error"))
				mock.ExpectRollback()
			},
			wantErr: fmt.Errorf("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			repo := NewGormGuildRepository(db)
			tt.mock(mock)

			err := repo.Delete(context.Background(), guildID)

			if tt.wantErr != nil {
				assert.Error(t, err)
				if tt.wantErr == domain.ErrHasRelatedRecords {
					assert.ErrorIs(t, err, domain.ErrHasRelatedRecords)
				}
			} else {
				assert.NoError(t, err)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGormGuildAttendeeRepository_GetByID(t *testing.T) {
	id := uuid.New()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr error
	}{
		{
			name: "Success",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE id = $1 ORDER BY "guild_attendee"."id" LIMIT $2`)).
					WithArgs(id, 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "role"}).
						AddRow(id, uuid.New(), uuid.New(), domain.GuildAttendeeRoleMember))
			},
		},
		{
			name: "NotFound",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE id = $1 ORDER BY "guild_attendee"."id" LIMIT $2`)).
					WithArgs(id, 1).
					WillReturnError(gorm.ErrRecordNotFound)
			},
			wantErr: domain.ErrNotFound,
		},
		{
			name: "DBError",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE id = $1 ORDER BY "guild_attendee"."id" LIMIT $2`)).
					WithArgs(id, 1).
					WillReturnError(fmt.Errorf("db error"))
			},
			wantErr: fmt.Errorf("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			repo := NewGormGuildAttendeeRepository(db)
			tt.mock(mock)

			attendee, err := repo.GetByID(context.Background(), id)

			if tt.wantErr != nil {
				assert.Error(t, err)
				if tt.wantErr == domain.ErrNotFound {
					assert.ErrorIs(t, err, domain.ErrNotFound)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, id, attendee.ID)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGormGuildAttendeeRepository_GetByGuildID(t *testing.T) {
	guildID := uuid.New()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr bool
		count   int
	}{
		{
			name: "Success",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE guild_id = $1`)).
					WithArgs(guildID).
					WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "role"}).
						AddRow(uuid.New(), guildID, uuid.New(), domain.GuildAttendeeRoleMember).
						AddRow(uuid.New(), guildID, uuid.New(), domain.GuildAttendeeRoleMaster))
			},
			count: 2,
		},
		{
			name: "EmptyResult",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE guild_id = $1`)).
					WithArgs(guildID).
					WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "role"}))
			},
			count: 0,
		},
		{
			name: "DBError",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE guild_id = $1`)).
					WithArgs(guildID).
					WillReturnError(fmt.Errorf("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			repo := NewGormGuildAttendeeRepository(db)
			tt.mock(mock)

			attendees, err := repo.GetByGuildID(context.Background(), guildID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, attendees, tt.count)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGormGuildAttendeeRepository_GetByUserID(t *testing.T) {
	userID := uuid.New()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr bool
		count   int
	}{
		{
			name: "Success",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE user_id = $1`)).
					WithArgs(userID).
					WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "role"}).
						AddRow(uuid.New(), uuid.New(), userID, domain.GuildAttendeeRoleMember))
			},
			count: 1,
		},
		{
			name: "EmptyResult",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE user_id = $1`)).
					WithArgs(userID).
					WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "role"}))
			},
			count: 0,
		},
		{
			name: "DBError",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE user_id = $1`)).
					WithArgs(userID).
					WillReturnError(fmt.Errorf("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			repo := NewGormGuildAttendeeRepository(db)
			tt.mock(mock)

			attendees, err := repo.GetByUserID(context.Background(), userID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, attendees, tt.count)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGormGuildAttendeeRepository_Create(t *testing.T) {
	tests := []struct {
		name     string
		attendee *domain.GuildAttendee
		mock     func(mock sqlmock.Sqlmock)
		wantErr  error
	}{
		{
			name: "Success",
			attendee: &domain.GuildAttendee{
				GuildID: uuid.New(),
				UserID:  uuid.New(),
				Role:    domain.GuildAttendeeRoleMember,
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild_attendee"`)).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
		},
		{
			name: "DuplicateEntry",
			attendee: &domain.GuildAttendee{
				GuildID: uuid.New(),
				UserID:  uuid.New(),
				Role:    domain.GuildAttendeeRoleMember,
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild_attendee"`)).
					WillReturnError(&pgconn.PgError{Code: "23505"})
				mock.ExpectRollback()
			},
			wantErr: domain.ErrDuplicateEntry,
		},
		{
			name: "DBError",
			attendee: &domain.GuildAttendee{
				GuildID: uuid.New(),
				UserID:  uuid.New(),
				Role:    domain.GuildAttendeeRoleMember,
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild_attendee"`)).
					WillReturnError(fmt.Errorf("db error"))
				mock.ExpectRollback()
			},
			wantErr: fmt.Errorf("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			repo := NewGormGuildAttendeeRepository(db)
			tt.mock(mock)

			err := repo.Create(context.Background(), tt.attendee)

			if tt.wantErr != nil {
				assert.Error(t, err)
				if tt.wantErr == domain.ErrDuplicateEntry {
					assert.ErrorIs(t, err, domain.ErrDuplicateEntry)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEqual(t, uuid.Nil, tt.attendee.ID)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGormGuildAttendeeRepository_Update(t *testing.T) {
	id := uuid.New()

	tests := []struct {
		name     string
		attendee *domain.GuildAttendee
		mock     func(mock sqlmock.Sqlmock)
		wantErr  bool
	}{
		{
			name: "Success",
			attendee: &domain.GuildAttendee{
				ID:   id,
				Role: domain.GuildAttendeeRoleMaster,
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE "guild_attendee"`)).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
		},
		{
			name: "DBError",
			attendee: &domain.GuildAttendee{
				ID:   id,
				Role: domain.GuildAttendeeRoleMaster,
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE "guild_attendee"`)).
					WillReturnError(fmt.Errorf("db error"))
				mock.ExpectRollback()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			repo := NewGormGuildAttendeeRepository(db)
			tt.mock(mock)

			err := repo.Update(context.Background(), tt.attendee)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGormGuildAttendeeRepository_Delete(t *testing.T) {
	id := uuid.New()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr bool
	}{
		{
			name: "Success",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "guild_attendee" WHERE id = $1`)).
					WithArgs(id).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			},
		},
		{
			name: "DBError",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "guild_attendee" WHERE id = $1`)).
					WithArgs(id).
					WillReturnError(fmt.Errorf("db error"))
				mock.ExpectRollback()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			repo := NewGormGuildAttendeeRepository(db)
			tt.mock(mock)

			err := repo.Delete(context.Background(), id)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGormGuildAttendeeRepository_GetByGuildAndUser(t *testing.T) {
	guildID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr error
	}{
		{
			name: "Success",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE guild_id = $1 AND user_id = $2 ORDER BY "guild_attendee"."id" LIMIT $3`)).
					WithArgs(guildID, userID, 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "role"}).
						AddRow(uuid.New(), guildID, userID, domain.GuildAttendeeRoleMember))
			},
		},
		{
			name: "NotFound",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE guild_id = $1 AND user_id = $2 ORDER BY "guild_attendee"."id" LIMIT $3`)).
					WithArgs(guildID, userID, 1).
					WillReturnError(gorm.ErrRecordNotFound)
			},
			wantErr: domain.ErrNotFound,
		},
		{
			name: "DBError",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE guild_id = $1 AND user_id = $2 ORDER BY "guild_attendee"."id" LIMIT $3`)).
					WithArgs(guildID, userID, 1).
					WillReturnError(fmt.Errorf("db error"))
			},
			wantErr: fmt.Errorf("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := setupMockDB(t)
			repo := NewGormGuildAttendeeRepository(db)
			tt.mock(mock)

			attendee, err := repo.GetByGuildAndUser(context.Background(), guildID, userID)

			if tt.wantErr != nil {
				assert.Error(t, err)
				if tt.wantErr == domain.ErrNotFound {
					assert.ErrorIs(t, err, domain.ErrNotFound)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, attendee)
				assert.Equal(t, guildID, attendee.GuildID)
				assert.Equal(t, userID, attendee.UserID)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
