// Package repository implements the domain repositories on top of GORM.
//
// Cross-repository "same transaction" writes must go through Store.Tx (spec
// §1.4): it hands the callback a RepoSet whose repositories all share one
// *gorm.DB transaction, and rolls every write back when the callback returns an
// error, panics, or runs with a cancelled context. A repository built on the
// root *gorm.DB (the one API holds for single-repository handlers) must never
// be combined with another repository to express one business operation.
//
// Worker enqueue is not part of a transaction (spec §1.7): enqueue only after
// Store.Tx has returned nil, never inside the callback.
package repository

import (
	"context"

	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

// RepoSet holds every domain repository, each bound to the same database
// transaction. A RepoSet is only ever handed out by Store.Tx; the zero value
// and direct construction are not supported.
//
// A RepoSet, and every repository in it, is only valid inside the callback
// that received it. Do not keep it, return it, or use it from a goroutine that
// outlives Store.Tx: once Tx returns the transaction is committed or rolled
// back, and a later call fails with sql.ErrTxDone.
//
// Repositories added by later issues extend this struct in their own issue.
type RepoSet struct {
	UserRepo          domain.UserRepository
	GuildRepo         domain.GuildRepository
	GuildAttendeeRepo domain.GuildAttendeeRepository
	EventRepo         domain.EventRepository
	EventAttendeeRepo domain.EventAttendeeRepository
	TranscriptRepo    domain.TranscriptRepository
	MistakeRepo       domain.MistakeRepository
}

// Store is the unit-of-work entry point for the backend. It owns the root
// *gorm.DB and opens the transactions used by cross-repository operations.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store writing through db. db is the root handle, not a
// transaction.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Tx runs fn inside one database transaction bound to ctx and returns fn's
// error unchanged.
//
// The transaction commits only when fn returns nil, the context is still alive,
// and the commit succeeds. An error or panic from fn rolls back every write made
// through the RepoSet; a panic is re-raised after the rollback. Errors that
// repositories already mapped to domain sentinels (for example
// domain.ErrDuplicateEntry) reach the caller untouched, so callers can keep
// using errors.Is.
//
// A repository method that opens its own transaction (CreateWithMaster,
// TransferLeader, CreateInviteLinkWithTx, Delete) joins this transaction as a
// GORM savepoint when it is called on a RepoSet repository, so its writes are
// rolled back with the rest of fn's work.
func (s *Store) Tx(ctx context.Context, fn func(r RepoSet) error) error {
	var fnErr error

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fnErr = fn(newRepoSet(tx))
		if fnErr != nil {
			return fnErr
		}

		// GORM commits whenever this callback returns nil, so report a dead
		// context here: an aborted request must not persist its writes.
		return ctx.Err()
	})

	if fnErr != nil {
		// Return the callback's error as is instead of remapping it, so a
		// domain sentinel never turns into a different error on the way out.
		return fnErr
	}

	// Begin or commit failure (a cancelled context, a connection error, or a
	// constraint PostgreSQL only checks at commit time).
	return MapGormError(err)
}

// newRepoSet builds one repository per domain repository on the same tx handle.
func newRepoSet(tx *gorm.DB) RepoSet {
	return RepoSet{
		UserRepo:          NewGormUserRepository(tx),
		GuildRepo:         NewGormGuildRepository(tx),
		GuildAttendeeRepo: NewGormGuildAttendeeRepository(tx),
		EventRepo:         NewGormEventRepository(tx),
		EventAttendeeRepo: NewGormEventAttendeeRepository(tx),
		TranscriptRepo:    NewGormTranscriptRepository(tx),
		MistakeRepo:       NewGormMistakeRepository(tx),
	}
}
