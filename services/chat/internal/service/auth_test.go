package service

import (
	auth "chatflow/gen/auth_service/v1"
	appErrors "chatflow/services/chat/internal/app-errors"
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type authConnection struct {
	grpc.ClientConnInterface
	err error
}

func (c authConnection) Invoke(_ context.Context, _ string, _ any, reply any, _ ...grpc.CallOption) error {
	if c.err != nil {
		return c.err
	}
	switch response := reply.(type) {
	case *auth.TokenResponse:
		response.UserID = 42
	case *auth.UserByIDResponse:
		response.Id = 42
	}
	return nil
}

func TestCheckTokenAuthResponse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rpcErr  error
		wantErr error
	}{
		{name: "valid token"},
		{name: "expired token", rpcErr: status.Error(codes.Unauthenticated, "expired"), wantErr: appErrors.ErrInvalidToken},
		{name: "auth unavailable", rpcErr: status.Error(codes.Unavailable, "offline")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(nil, authConnection{err: tc.rpcErr})
			id, err := s.CheckToken(context.Background(), "token")
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got %v, want %v", err, tc.wantErr)
				}
			} else if tc.rpcErr != nil {
				if status.Code(err) != codes.Unavailable {
					t.Fatalf("auth outage lost: %v", err)
				}
			} else if err != nil || id != 42 {
				t.Fatalf("got user %d, error %v", id, err)
			}
		})
	}
}

func TestCreateGroupValidatesMembersThroughAuth(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rpcErr  error
		wantErr error
	}{
		{name: "unknown member", rpcErr: status.Error(codes.NotFound, "user missing"), wantErr: appErrors.ErrUnknownMembers},
		{name: "auth unavailable", rpcErr: status.Error(codes.Unavailable, "offline")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(nil, authConnection{err: tc.rpcErr})
			_, err := s.CreateGroup(context.Background(), GroupCreate{Name: "group", OwnerID: 1, Members: []int{42}})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got %v, want %v", err, tc.wantErr)
				}
			} else if status.Code(err) != codes.Unavailable {
				t.Fatalf("auth outage lost: %v", err)
			}
		})
	}
}

func TestDirectChatRejectsUnknownPeer(t *testing.T) {
	s := New(nil, authConnection{err: status.Error(codes.NotFound, "user missing")})
	_, _, err := s.GetOrCreateChat(context.Background(), 42, 1)
	if !errors.Is(err, appErrors.ErrUserDoesntExist) {
		t.Fatalf("got %v, want unknown user", err)
	}
}
