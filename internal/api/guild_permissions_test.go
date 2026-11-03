package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/domain"
)

type guildOperationsStub struct {
	domain.GuildApplicationRepository
	app   domain.GuildApplication
	calls int
}

func (s *guildOperationsStub) ListPendingByGuildID(context.Context, uuid.UUID) ([]*domain.GuildApplication, error) {
	s.calls++
	return []*domain.GuildApplication{&s.app}, nil
}
func (s *guildOperationsStub) GetByID(context.Context, uuid.UUID) (*domain.GuildApplication, error) {
	s.calls++
	return &s.app, nil
}
func (s *guildOperationsStub) ApproveWithTx(context.Context, *domain.GuildApplication, *domain.GuildAttendee) error {
	s.calls++
	return nil
}
func (s *guildOperationsStub) RejectPending(context.Context, uuid.UUID, uuid.UUID) error {
	s.calls++
	return nil
}

type slotOperationsStub struct {
	domain.GuildDefaultSlotRepository
	slot  *domain.GuildDefaultSlot
	calls int
}

func (s *slotOperationsStub) Upsert(_ context.Context, slot *domain.GuildDefaultSlot) error {
	s.calls++
	s.slot = slot
	return nil
}
func (s *slotOperationsStub) GetByGuildID(context.Context, uuid.UUID) (*domain.GuildDefaultSlot, error) {
	s.calls++
	return s.slot, nil
}
func (s *slotOperationsStub) DeleteByGuildID(context.Context, uuid.UUID) error {
	s.calls++
	return nil
}

func TestGuildMasterOperationsPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	left := time.Now()
	for _, operation := range []string{"list", "approve", "reject", "put slot", "delete slot", "invite"} {
		for _, test := range []struct {
			name        string
			caller      any
			guildErr    error
			attendee    *domain.GuildAttendee
			attendeeErr error
			status      int
		}{
			{name: "missing caller", status: 401},
			{name: "invalid context type", caller: "invalid", status: 500},
			{name: "deleted guild", caller: uuid.New(), guildErr: domain.ErrNotFound, status: 404},
			{name: "guild query error", caller: uuid.New(), guildErr: errors.New("db unavailable"), status: 500},
			{name: "nonmember", caller: uuid.New(), attendeeErr: domain.ErrNotFound, status: 403},
			{name: "member", caller: uuid.New(), attendee: &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMember}, status: 403},
			{name: "departed master", caller: uuid.New(), attendee: &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMaster, LeftAt: &left}, status: 403},
			{name: "membership query error", caller: uuid.New(), attendeeErr: errors.New("db unavailable"), status: 500},
			{name: "active master", caller: uuid.New(), attendee: &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMaster}, status: 200},
		} {
			t.Run(operation+"/"+test.name, func(t *testing.T) {
				api, guilds, members := newTestAPI()
				guildID, appID := uuid.New(), uuid.New()
				guilds.getByIDResult = &domain.Guild{ID: guildID}
				guilds.getByIDErr = test.guildErr
				members.getByGuildAndUserResult, members.getByGuildAndUserErr = test.attendee, test.attendeeErr
				apps := &guildOperationsStub{app: domain.GuildApplication{ID: appID, GuildID: guildID, Status: domain.GuildApplicationStatusPending}}
				slots := &slotOperationsStub{}
				api.applicationRepo, api.defaultSlotRepo = apps, slots
				handlers := map[string]gin.HandlerFunc{
					"list": api.GuildApplicationsHandler, "approve": api.GuildApplicationApproveHandler,
					"reject": api.GuildApplicationRejectHandler, "put slot": api.GuildDefaultSlotUpsertHandler,
					"delete slot": api.GuildDefaultSlotDeleteHandler, "invite": api.GuildInviteLinkCreateHandler,
				}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Params = gin.Params{{Key: "id", Value: guildID.String()}, {Key: "app_id", Value: appID.String()}}
				c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"day_of_week":0,"start_time":"09:00","end_time":"10:00"}`))
				c.Request.Header.Set("Content-Type", "application/json")
				if test.caller != nil {
					c.Set("userID", test.caller)
				}
				handlers[operation](c)
				require.Equal(t, test.status, recorder.Code, recorder.Body.String())
				calls := apps.calls + slots.calls + len(guilds.createInviteLinkCalls)
				if test.status == 200 {
					require.Positive(t, calls)
				} else {
					require.Zero(t, calls, "denied requests must not access protected data")
				}
			})
		}
	}
}

func TestGuildDefaultSlotTimeValidation(t *testing.T) {
	for _, test := range []struct {
		start, end string
		status     int
	}{
		{"09:00", "10:00", 200}, {"00:00", "23:59", 200},
		{"09:00:00", "10:00", 400}, {"zz", "10:00", 400},
		{"9:00", "10:00", 400}, {"24:00", "10:00", 400},
		{"09:60", "10:00", 400}, {"", "10:00", 400},
		{"09:00", "10:00:00", 400}, {"09:00", "zz", 400},
		{"09:00", "09:00", 400}, {"23:00", "01:00", 400},
	} {
		t.Run(test.start+"-"+test.end, func(t *testing.T) {
			api, _, members := newTestAPI()
			members.getByGuildAndUserResult = &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMaster}
			slots := &slotOperationsStub{}
			api.defaultSlotRepo = slots
			caller := uuid.New()
			body := `{"day_of_week":0,"start_time":"` + test.start + `","end_time":"` + test.end + `"}`
			rec := invokeHandler(t, api.GuildDefaultSlotUpsertHandler, "/guilds/"+uuid.NewString()+"/default-slot", &caller, []byte(body))
			require.Equal(t, test.status, rec.Code, rec.Body.String())
			if test.status == 400 {
				require.Zero(t, slots.calls)
			}
		})
	}
}

type discoverGuildStub struct {
	domain.GuildRepository
	page, size, calls int
}

func (s *discoverGuildStub) Discover(_ context.Context, page, size int) (*domain.GuildDiscoverResult, error) {
	s.page, s.size = page, size
	s.calls++
	return &domain.GuildDiscoverResult{Page: page, PageSize: size}, nil
}

func TestGuildDiscoverPaginationValidation(t *testing.T) {
	for _, test := range []struct {
		query              string
		status, page, size int
	}{
		{"", 200, 1, 10}, {"?page=2&page_size=100", 200, 2, 100},
		{"?page=0", 400, 0, 0}, {"?page=-1", 400, 0, 0},
		{"?page_size=0", 400, 0, 0}, {"?page_size=101", 400, 0, 0},
	} {
		t.Run(test.query, func(t *testing.T) {
			repo := &discoverGuildStub{}
			api := &API{guildRepo: repo}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/discover"+test.query, nil)
			api.GuildDiscoverHandler(c)
			require.Equal(t, test.status, rec.Code)
			if test.status == 200 {
				require.Equal(t, test.page, repo.page)
				require.Equal(t, test.size, repo.size)
			} else {
				require.Zero(t, repo.calls)
			}
		})
	}
}
