package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"jpcorrect-backend/internal/domain"
)

type gormUserRepository struct {
	db *gorm.DB
}

// NewGormUserRepository creates a new GORM-based user repository.
func NewGormUserRepository(db *gorm.DB) domain.UserRepository {
	return &gormUserRepository{db: db}
}

func (r *gormUserRepository) GetByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).First(&user, "id = ?", userID).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &user, nil
}

func (r *gormUserRepository) GetByName(ctx context.Context, name string) ([]*domain.User, error) {
	var users []*domain.User
	err := r.db.WithContext(ctx).Where("name = ?", name).Find(&users).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return users, nil
}

func (r *gormUserRepository) GetBySupabaseUserID(ctx context.Context, supabaseUserID uuid.UUID) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Where("supabase_user_id = ?", supabaseUserID).First(&user).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &user, nil
}

// CreateIfAbsentBySupabaseUserID is the §6.3 idempotent insert: one
// `INSERT … ON CONFLICT (supabase_user_id) DO NOTHING`. Only a conflict on
// supabase_user_id is ignored; a concurrent request for the same sub blocks on
// the unique index and then writes nothing, so every caller reads the same row
// afterwards. Any other unique violation (e.g. the email index) still fails and
// reaches the caller as domain.ErrDuplicateEntry.
func (r *gormUserRepository) CreateIfAbsentBySupabaseUserID(ctx context.Context, user *domain.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "supabase_user_id"}},
			DoNothing: true,
		}).
		Create(user).Error)
}

func (r *gormUserRepository) Create(ctx context.Context, user *domain.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).Create(user).Error)
}

func (r *gormUserRepository) Update(ctx context.Context, user *domain.User) error {
	return MapGormError(r.db.WithContext(ctx).Save(user).Error)
}

// UpdateProfile writes only the owner-editable profile columns of one live row
// (spec §3.1.2) and returns the row as PostgreSQL stored it.
//
// The statement names the columns it writes, so every server-owned column
// (email, role, status, points, supabase_user_id, is_email_verified, timezone,
// level) keeps whatever the last committed write left there. A caller that
// loaded the row earlier and saved it back would instead restore its own
// snapshot, which loses a concurrent role change, a ban or a points award.
//
// RETURNING * gives the caller the committed row in the same statement: the
// response carries the allowlisted fields this request wrote plus every other
// column as of that statement, without a second read and without a transaction.
func (r *gormUserRepository) UpdateProfile(ctx context.Context, userID uuid.UUID, name *string, avatarURL *string) (*domain.User, error) {
	if userID == uuid.Nil {
		return nil, domain.ErrNotFound
	}

	// Name and avatar_url are the only columns in the SET list. GORM adds
	// updated_at for a map update on a model that has the column, and the row
	// did change.
	columns := make(map[string]any, 2)
	if name != nil {
		columns["name"] = *name
	}
	if avatarURL != nil {
		columns["avatar_url"] = *avatarURL
	}

	if len(columns) == 0 {
		// Nothing was requested: no write, so updated_at does not move either.
		return r.GetByID(ctx, userID)
	}

	// The model carries no primary key, so the explicit predicate is the whole
	// filter and RETURNING scans the stored row into stored.
	stored := domain.User{}
	result := r.db.WithContext(ctx).
		Model(&stored).
		Clauses(clause.Returning{}).
		Where("id = ?", userID).
		Updates(columns)
	if result.Error != nil {
		return nil, MapGormError(result.Error)
	}

	if result.RowsAffected == 0 {
		// No live row carries this id: it never existed, or it is soft-deleted.
		return nil, domain.ErrNotFound
	}

	return &stored, nil
}

func (r *gormUserRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	// GORM soft delete
	return MapGormError(r.db.WithContext(ctx).Delete(&domain.User{}, "id = ?", userID).Error)
}
