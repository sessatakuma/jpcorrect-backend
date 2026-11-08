package api

import (
	"context"
	"errors"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// This file centralizes the spec §1.3 permission checks. Every endpoint and
// WebSocket subscription labels itself with one of the six markers and calls
// the matching helper instead of querying membership or ownership itself.
//
// Each check exists twice:
//
//   - Permissions is the core rule. It takes the caller, the ids and the
//     repositories, never a *gin.Context, so the WebSocket subscribe path
//     (#80) can run the same checks for the user bound to a connection.
//   - Require* is the thin gin wrapper the handlers call. It resolves the
//     caller, runs the core rule, writes the §1.5 envelope itself when the
//     rule fails, and returns the error so the handler can return immediately:
//
//	func (a *API) ReviewEndHandler(c *gin.Context) {
//		activityID := /* parsed from the path */
//		if err := a.RequireActivityHost(c, activityID); err != nil {
//			return // the helper already wrote the envelope
//		}
//		// ...the handler's own work and its single success response
//	}
//
// A missing or not-initialized caller is always a 401, a missing target is a
// 404 and a failed rule is a 403, all through writeError.
//
// AUTH is AuthMiddleware (internal/api/auth.go); this file owns the other five
// markers, including the two record-scoped OWNER checks.
//
// There is no global admin bypass (§1.3, maintainer clarification): a User with
// Role admin or staff is subject to the ordinary membership, leader, ownership
// and host rules exactly like anyone else. LEADER means
// GuildAttendee.Role == master in that guild, never User.Role.
//
// A member who left (GuildAttendee.LeftAt IS NOT NULL) is not a member:
// GuildAttendeeRepository.GetByGuildAndUser only returns active rows.

// Permissions is the core of the §1.3 permission checks: it runs them without a
// *gin.Context. All fields are interfaces so a test can substitute a fake, and
// so #80 can hand in the repositories it already has.
type Permissions struct {
	guildRepo            domain.GuildRepository
	guildAttendeeRepo    domain.GuildAttendeeRepository
	activityLookupRepo   domain.ActivityLookupRepository
	mistakeRepo          domain.MistakeRepository
	selectedSentenceRepo domain.SelectedSentenceLookupRepository
}

// NewPermissions builds the core checker.
func NewPermissions(
	guildRepo domain.GuildRepository,
	guildAttendeeRepo domain.GuildAttendeeRepository,
	activityLookupRepo domain.ActivityLookupRepository,
	mistakeRepo domain.MistakeRepository,
	selectedSentenceRepo domain.SelectedSentenceLookupRepository,
) *Permissions {
	return &Permissions{
		guildRepo:            guildRepo,
		guildAttendeeRepo:    guildAttendeeRepo,
		activityLookupRepo:   activityLookupRepo,
		mistakeRepo:          mistakeRepo,
		selectedSentenceRepo: selectedSentenceRepo,
	}
}

// CheckGuildMember is the GUILD marker: the caller is an active member of
// guildID.
func (p *Permissions) CheckGuildMember(ctx context.Context, caller *domain.User, guildID uuid.UUID) error {
	_, err := p.activeMembership(ctx, caller, guildID)
	return err
}

// CheckGuildLeader is the LEADER marker: the caller is an active member of
// guildID whose GuildAttendee.Role is master. Leadership is guild-scoped, so
// leading another guild grants nothing here.
func (p *Permissions) CheckGuildLeader(ctx context.Context, caller *domain.User, guildID uuid.UUID) error {
	attendee, err := p.activeMembership(ctx, caller, guildID)
	if err != nil {
		return err
	}
	if attendee.Role != domain.GuildAttendeeRoleMaster {
		return errorWithMessage(domain.ErrNotGuildMaster, "caller is not the leader of this guild")
	}
	return nil
}

// CheckActivityMember is the ACTIVITY marker: the Activity belongs to a guild
// the caller is an active member of.
func (p *Permissions) CheckActivityMember(ctx context.Context, caller *domain.User, activityID uuid.UUID) error {
	activity, err := p.activity(ctx, caller, activityID)
	if err != nil {
		return err
	}
	return p.CheckGuildMember(ctx, caller, activity.GuildID)
}

// CheckSelfOrGuildLeader is the OWNER marker for a user-scoped target: the
// caller acts on themself, or as the leader of guildID on that member's behalf.
// Without a guild there is nobody who may act for another user.
func (p *Permissions) CheckSelfOrGuildLeader(ctx context.Context, caller *domain.User, targetUserID uuid.UUID, guildID uuid.UUID) error {
	if caller == nil {
		return domain.ErrUnauthorized
	}
	if caller.ID == targetUserID {
		return nil
	}
	if guildID == uuid.Nil {
		return errorWithMessage(domain.ErrForbidden, "caller cannot act for another user outside a guild")
	}
	return p.CheckGuildLeader(ctx, caller, guildID)
}

// CheckMistakeOwner is the OWNER marker for a mistake: the mistake's user_id is
// the caller (§3.11.2). Not being the owner is the catalog's NOT_MISTAKE_OWNER;
// a leader or an admin has no standing either.
func (p *Permissions) CheckMistakeOwner(ctx context.Context, caller *domain.User, mistakeID uuid.UUID) error {
	if caller == nil {
		return domain.ErrUnauthorized
	}

	mistake, err := p.mistakeRepo.GetByID(ctx, mistakeID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errorWithMessage(domain.ErrNotFound, "mistake not found")
		}
		return err
	}

	if mistake.UserID != caller.ID {
		return errorWithMessage(domain.ErrNotMistakeOwner, "caller is not the owner of this mistake")
	}
	return nil
}

// CheckSelectedSentenceOwner is the OWNER marker for a selected sentence: the
// selection's selected_by_user_id is the caller (§3.11.2). §1.5 has no code for
// this case, so it is the generic 403.
func (p *Permissions) CheckSelectedSentenceOwner(ctx context.Context, caller *domain.User, selectedSentenceID uuid.UUID) error {
	if caller == nil {
		return domain.ErrUnauthorized
	}

	selected, err := p.selectedSentenceRepo.GetByID(ctx, selectedSentenceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errorWithMessage(domain.ErrNotFound, "selected sentence not found")
		}
		return err
	}

	if selected.SelectedByUserID != caller.ID {
		return errorWithMessage(domain.ErrForbidden, "caller is not the owner of this selected sentence")
	}
	return nil
}

// activeMembership loads guildID and the caller's active GuildAttendee row in
// it. A missing (or soft-deleted) guild is a 404; an absent active row means
// the caller never joined or has left, which is a 403.
func (p *Permissions) activeMembership(ctx context.Context, caller *domain.User, guildID uuid.UUID) (*domain.GuildAttendee, error) {
	if caller == nil {
		return nil, domain.ErrUnauthorized
	}

	if _, err := p.guildRepo.GetByID(ctx, guildID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errorWithMessage(domain.ErrNotFound, "guild not found")
		}
		return nil, err
	}

	attendee, err := p.guildAttendeeRepo.GetByGuildAndUser(ctx, guildID, caller.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errorWithMessage(domain.ErrNotGuildMember, "caller is not an active member of this guild")
		}
		return nil, err
	}

	return attendee, nil
}

// activity loads activityID for the permission checks. A missing or
// soft-deleted Activity is a 404.
func (p *Permissions) activity(ctx context.Context, caller *domain.User, activityID uuid.UUID) (*domain.Activity, error) {
	if caller == nil {
		return nil, domain.ErrUnauthorized
	}

	activity, err := p.activityLookupRepo.GetByID(ctx, activityID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errorWithMessage(domain.ErrNotFound, "activity not found")
		}
		return nil, err
	}

	return activity, nil
}

// permissions builds the core checker around the API's repositories. The two
// lookups below are stateless single reads, so they are created from the same
// root handle here rather than cached on API.
func (a *API) permissions() *Permissions {
	return NewPermissions(
		a.guildRepo,
		a.guildAttendeeRepo,
		repository.NewGormActivityLookupRepository(a.db),
		a.mistakeRepo,
		repository.NewGormSelectedSentenceLookupRepository(a.db),
	)
}

// requirePermission runs one core check for the request's caller, writing the
// §1.5 envelope on failure so the handler only has to `return`.
func (a *API) requirePermission(c *gin.Context, check func(*Permissions, *domain.User) error) error {
	caller := currentUser(c)
	if caller == nil {
		writeError(c, domain.ErrUnauthorized)
		return domain.ErrUnauthorized
	}

	if err := check(a.permissions(), caller); err != nil {
		writeError(c, err)
		return err
	}

	return nil
}

// RequireGuildMember enforces the GUILD marker (§1.3).
func (a *API) RequireGuildMember(c *gin.Context, guildID uuid.UUID) error {
	return a.requirePermission(c, func(p *Permissions, caller *domain.User) error {
		return p.CheckGuildMember(c.Request.Context(), caller, guildID)
	})
}

// RequireGuildLeader enforces the LEADER marker (§1.3).
func (a *API) RequireGuildLeader(c *gin.Context, guildID uuid.UUID) error {
	return a.requirePermission(c, func(p *Permissions, caller *domain.User) error {
		return p.CheckGuildLeader(c.Request.Context(), caller, guildID)
	})
}

// RequireActivityMember enforces the ACTIVITY marker (§1.3).
func (a *API) RequireActivityMember(c *gin.Context, activityID uuid.UUID) error {
	return a.requirePermission(c, func(p *Permissions, caller *domain.User) error {
		return p.CheckActivityMember(c.Request.Context(), caller, activityID)
	})
}

// RequireSelfOrGuildLeader enforces the OWNER marker for a user-scoped target
// (§1.3).
func (a *API) RequireSelfOrGuildLeader(c *gin.Context, targetUserID uuid.UUID, guildID uuid.UUID) error {
	return a.requirePermission(c, func(p *Permissions, caller *domain.User) error {
		return p.CheckSelfOrGuildLeader(c.Request.Context(), caller, targetUserID, guildID)
	})
}

// RequireMistakeOwner enforces the OWNER marker for a mistake (§1.3, §3.11.2).
func (a *API) RequireMistakeOwner(c *gin.Context, mistakeID uuid.UUID) error {
	return a.requirePermission(c, func(p *Permissions, caller *domain.User) error {
		return p.CheckMistakeOwner(c.Request.Context(), caller, mistakeID)
	})
}

// RequireSelectedSentenceOwner enforces the OWNER marker for a selection
// (§1.3, §3.11.2).
func (a *API) RequireSelectedSentenceOwner(c *gin.Context, selectedSentenceID uuid.UUID) error {
	return a.requirePermission(c, func(p *Permissions, caller *domain.User) error {
		return p.CheckSelectedSentenceOwner(c.Request.Context(), caller, selectedSentenceID)
	})
}
