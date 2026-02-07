package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"jpcorrect-backend/internal/domain"
)

func setupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	})
	if err != nil {
		t.Fatalf("failed to open gorm: %v", err)
	}

	return gormDB, mock
}

func TestGormUserRepository_GetByID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormUserRepository(db)
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user" WHERE id = $1 AND "user"."deleted_at" IS NULL ORDER BY "user"."id" LIMIT $2`)).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name"}).
				AddRow(userID, "test@example.com", "Test User"))

		user, err := repo.GetByID(context.Background(), userID)

		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, userID, user.ID)
		assert.Equal(t, "test@example.com", user.Email)
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user" WHERE id = $1 AND "user"."deleted_at" IS NULL ORDER BY "user"."id" LIMIT $2`)).
			WithArgs(userID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		user, err := repo.GetByID(context.Background(), userID)

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, user)
	})
}

func TestGormUserRepository_GetByEmail(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormUserRepository(db)
	email := "test@example.com"

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user" WHERE email = $1 AND "user"."deleted_at" IS NULL ORDER BY "user"."id" LIMIT $2`)).
			WithArgs(email, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name"}).
				AddRow(uuid.New(), email, "Test User"))

		user, err := repo.GetByEmail(context.Background(), email)

		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, email, user.Email)
	})
}

func TestGormUserRepository_GetByName(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormUserRepository(db)
	name := "Test User"

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user" WHERE name = $1 AND "user"."deleted_at" IS NULL`)).
			WithArgs(name).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name"}).
				AddRow(uuid.New(), "test1@example.com", name).
				AddRow(uuid.New(), "test2@example.com", name))

		users, err := repo.GetByName(context.Background(), name)

		assert.NoError(t, err)
		assert.Len(t, users, 2)
	})
}

func TestGormUserRepository_Create(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormUserRepository(db)

	t.Run("Success", func(t *testing.T) {
		user := &domain.User{
			Email: "test@example.com",
			Name:  "Test User",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "user"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Create(context.Background(), user)

		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, user.ID)
	})
}

func TestGormUserRepository_Update(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormUserRepository(db)
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: "updated@example.com",
			Name:  "Updated User",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "user"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Update(context.Background(), user)

		assert.NoError(t, err)
	})
}

func TestGormUserRepository_Delete(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormUserRepository(db)
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "user" SET "deleted_at"=$1 WHERE id = $2 AND "user"."deleted_at" IS NULL`)).
			WithArgs(sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Delete(context.Background(), userID)

		assert.NoError(t, err)
	})
}
