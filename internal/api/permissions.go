package api

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"
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
// The one exception is not a permission but a route gate: RequireSystemAdmin at
// the bottom of this file guards the retained legacy raw CRUD management routes
// (§3.15) and is never consulted by any check above.
//
// A member who left (GuildAttendee.LeftAt IS NOT NULL) is not a member:
// GuildAttendeeRepository.GetByGuildAndUser only returns active rows.

// §4.2 channel names, used to find the room whose host governs an Activity.
const (
	practiceChannelPrefix = "practice:"
	reviewChannelPrefix   = "review:"
)

// ActivityRoomChannel returns the §4.2 realtime channel whose host governs the
// Activity with this status, and false when the Activity has no room that
// carries a host.
//
// §1.3 leaves the channel selection to the implementation and §4.2 only names
// the two rooms, so this is the documented rule: an Activity is hosted in the
// room of the stage it is in — the practice room from scheduling until the
// analysis is done (pending_practice, in_practice, analyzing) and the review
// room afterwards (in_feedback, in_review). A terminal Activity (done,
// aborted) has no active room, so nobody is its host.
func ActivityRoomChannel(activityID uuid.UUID, status domain.ActivityStatus) (string, bool) {
	switch status {
	case domain.ActivityStatusPendingPractice,
		domain.ActivityStatusInPractice,
		domain.ActivityStatusAnalyzing:
		return practiceChannelPrefix + activityID.String(), true
	case domain.ActivityStatusInFeedback,
		domain.ActivityStatusInReview:
		return reviewChannelPrefix + activityID.String(), true
	default:
		return "", false
	}
}

// Permissions is the core of the §1.3 permission checks: it runs them without a
// *gin.Context. All fields are interfaces so a test can substitute a fake, and
// so #80 can hand in the repositories it already has.
type Permissions struct {
	guildRepo            domain.GuildRepository
	guildAttendeeRepo    domain.GuildAttendeeRepository
	activityLookupRepo   domain.ActivityLookupRepository
	mistakeRepo          domain.MistakeRepository
	selectedSentenceRepo domain.SelectedSentenceLookupRepository
	hostLookup           domain.WebRTCHub
}

// NewPermissions builds the core checker. hostLookup may be nil, in which case
// no room has a host.
func NewPermissions(
	guildRepo domain.GuildRepository,
	guildAttendeeRepo domain.GuildAttendeeRepository,
	activityLookupRepo domain.ActivityLookupRepository,
	mistakeRepo domain.MistakeRepository,
	selectedSentenceRepo domain.SelectedSentenceLookupRepository,
	hostLookup domain.WebRTCHub,
) *Permissions {
	return &Permissions{
		guildRepo:            guildRepo,
		guildAttendeeRepo:    guildAttendeeRepo,
		activityLookupRepo:   activityLookupRepo,
		mistakeRepo:          mistakeRepo,
		selectedSentenceRepo: selectedSentenceRepo,
		hostLookup:           hostLookup,
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

// CheckActivityHost is the HOST marker: the caller is the Hub host of the room
// the Activity is currently in (§4.2 through ActivityRoomChannel, rules in
// §4.5). A terminal Activity has no room and therefore no host.
func (p *Permissions) CheckActivityHost(ctx context.Context, caller *domain.User, activityID uuid.UUID) error {
	activity, err := p.activity(ctx, caller, activityID)
	if err != nil {
		return err
	}

	channel, ok := ActivityRoomChannel(activity.ID, activity.Status)
	if !ok {
		return errorWithMessage(domain.ErrForbidden, "activity has no active room with a host")
	}

	return p.CheckChannelHost(caller, channel)
}

// CheckChannelHost compares the Hub's host of channelID with the caller. It is
// the form #80 uses for a subscribe message, where the channel name is already
// known. No host, an unknown channel and a different host are all a 403.
func (p *Permissions) CheckChannelHost(caller *domain.User, channelID string) error {
	if caller == nil {
		return domain.ErrUnauthorized
	}

	if p.hostLookup != nil {
		if host, ok := p.hostLookup.GetHost(channelID); ok && host != uuid.Nil && host == caller.ID {
			return nil
		}
	}

	return errorWithMessage(domain.ErrForbidden, "caller is not the host of this room")
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

// permissions builds the core checker around the API's repositories and the
// live hub. The two lookups above are stateless single reads, so they are
// created from the same root handle here rather than cached on API, and the hub
// is read live so a replaced hub is always honoured.
func (a *API) permissions() *Permissions {
	return NewPermissions(
		a.guildRepo,
		a.guildAttendeeRepo,
		repository.NewGormActivityLookupRepository(a.db),
		a.mistakeRepo,
		repository.NewGormSelectedSentenceLookupRepository(a.db),
		a.webrtcHub,
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

// RequireActivityHost enforces the HOST marker (§1.3, §4.5). The channel comes
// from the Activity's current stage; see ActivityRoomChannel.
func (a *API) RequireActivityHost(c *gin.Context, activityID uuid.UUID) error {
	return a.requirePermission(c, func(p *Permissions, caller *domain.User) error {
		return p.CheckActivityHost(c.Request.Context(), caller, activityID)
	})
}

// IsSystemAdmin reports whether user is a system administrator: a caller whose
// trusted, server-resolved stored role is admin (spec §1.3 "System
// administration and legacy raw CRUD").
//
// Guild leadership (GuildAttendee.Role == master) is not system administration,
// staff does not imply it, and a role carried by a request body or by a custom
// JWT claim is never consulted: user is always the row AuthMiddleware resolved
// from the token's subject.
func IsSystemAdmin(user *domain.User) bool {
	return user != nil && user.Role == domain.UserRoleAdmin
}

// RequireSystemAdmin is the route-level gate of the retained legacy raw CRUD
// management routes (§1.3, §3.15). Mount it after AuthMiddleware on a route
// group or a single route; it allows only IsSystemAdmin(caller) and answers
// 401 for a request without a resolved caller and 403 for every other role, in
// every GIN_MODE, through the §1.5 envelope.
//
// It deliberately sits outside the six §1.3 markers above: it is a gate on the
// raw management API, not a product permission. No product helper consults it,
// so there is no global admin bypass, and it runs before the request body is
// read because it is middleware.
func (a *API) RequireSystemAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		caller := currentUser(c)
		if caller == nil {
			writeError(c, domain.ErrUnauthorized)
			c.Abort()
			return
		}

		if !IsSystemAdmin(caller) {
			writeError(c, errorWithMessage(domain.ErrForbidden, "system administrator role required"))
			c.Abort()
			return
		}

		c.Next()
	}
}
