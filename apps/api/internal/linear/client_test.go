package linear

import (
	"errors"
	"testing"
)

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "wrapped sentinel",
			err:  errors.Join(ErrNotAuthenticated, errors.New("extra context")),
			want: true,
		},
		{
			name: "auth message",
			err:  errors.New("Authentication required, not authenticated"),
			want: true,
		},
		{
			name: "invalid token message",
			err:  errors.New("invalid token provided"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("network timeout"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAuthError(tt.err); got != tt.want {
				t.Fatalf("IsAuthError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestAuthAwareError(t *testing.T) {
	err := authAwareError("linear issue create failed", "Authentication required, not authenticated")
	if !IsAuthError(err) {
		t.Fatalf("authAwareError must produce auth error for auth message")
	}

	nonAuth := authAwareError("linear issue create failed", "team not found")
	if IsAuthError(nonAuth) {
		t.Fatalf("authAwareError must not flag non-auth message")
	}
}
