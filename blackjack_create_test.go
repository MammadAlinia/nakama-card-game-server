package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/heroiclabs/nakama-common/runtime"
)

// Embedding the large runtime interface lets this fake implement only the API
// the creation RPC uses. Unexpected calls to other APIs will fail the test.
type createTestNakama struct {
	runtime.NakamaModule
	matchID string
	err     error
	calls   int
	module  string
	params  map[string]interface{}
}

func (n *createTestNakama) MatchCreate(ctx context.Context, module string, params map[string]interface{}) (string, error) {
	n.calls++
	n.module, n.params = module, params
	return n.matchID, n.err
}

func TestRpcBlackjackCreateAuthentication(t *testing.T) {
	tests := []struct {
		name string
		user interface{}
	}{
		{"missing identity", nil},
		{"empty identity", ""},
		{"wrong identity type", 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.user != nil {
				ctx = context.WithValue(ctx, runtime.RUNTIME_CTX_USER_ID, tt.user)
			}
			nk := &createTestNakama{matchID: "test-match-id"}
			response, err := RpcBlackjackCreate(ctx, nil, nil, nk, `{"owner_user_id":"claimed-owner"}`)
			var rpcError *runtime.Error
			if !errors.As(err, &rpcError) || rpcError.Code != 16 {
				t.Errorf("unauthenticated creation should return error code 16; got %v", err)
			}
			if response != "" || nk.calls != 0 {
				t.Error("unauthenticated requests must not create a match or return a success payload")
			}
		})
	}
}

func TestRpcBlackjackCreateOwnershipAndResponse(t *testing.T) {
	for _, payload := range []string{"", `{"owner_user_id":"claimed-owner"}`, "not JSON"} {
		t.Run("payload="+payload, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), runtime.RUNTIME_CTX_USER_ID, "authenticated-owner")
			nk := &createTestNakama{matchID: "test-match-id"}
			response, err := RpcBlackjackCreate(ctx, nil, nil, nk, payload)
			if err != nil {
				t.Fatalf("creation failed: %v", err)
			}
			if nk.calls != 1 || nk.module != "blackjack" {
				t.Errorf("should create one blackjack match; got %d calls to %q", nk.calls, nk.module)
			}
			if nk.params["owner_user_id"] != "authenticated-owner" {
				t.Errorf("owner should come from authentication, not payload; got %v", nk.params)
			}
			var body map[string]string
			if err := json.Unmarshal([]byte(response), &body); err != nil {
				t.Fatalf("response is not valid JSON: %v", err)
			}
			if body["match_id"] != "test-match-id" {
				t.Errorf("response match_id = %q; want test-match-id", body["match_id"])
			}
		})
	}
}

func TestRpcBlackjackCreateFailures(t *testing.T) {
	creationError := errors.New("match creation failed")
	tests := []struct {
		name    string
		matchID string
		err     error
	}{
		{"creation error", "", creationError},
		{"creation error with an ID", "test-match-id", creationError},
		{"empty match ID without an error", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), runtime.RUNTIME_CTX_USER_ID, "owner")
			nk := &createTestNakama{matchID: tt.matchID, err: tt.err}
			response, err := RpcBlackjackCreate(ctx, nil, nil, nk, "")
			if err == nil {
				t.Error("failed creation or a missing match ID must return a non-nil error")
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Errorf("creation error was not propagated; got %v", err)
			}
			if response != "" || nk.calls != 1 {
				t.Error("failed creation must not return a success payload or retry")
			}
		})
	}
}

type createTestRpc = func(context.Context, runtime.Logger, *sql.DB, runtime.NakamaModule, string) (string, error)
type createTestFactory = func(context.Context, runtime.Logger, *sql.DB, runtime.NakamaModule) (runtime.Match, error)

type createTestInitializer struct {
	runtime.Initializer
	rpcs    map[string]createTestRpc
	factory createTestFactory
	calls   []string
	fail    string
	err     error
}

func (i *createTestInitializer) RegisterRpc(id string, fn createTestRpc) error {
	i.calls = append(i.calls, "rpc:"+id)
	if i.fail == "rpc:"+id {
		return i.err
	}
	if i.rpcs == nil {
		i.rpcs = make(map[string]createTestRpc)
	}
	i.rpcs[id] = fn
	return nil
}

func (i *createTestInitializer) RegisterMatch(name string, fn createTestFactory) error {
	i.calls = append(i.calls, "match:"+name)
	if i.fail == "match:"+name {
		return i.err
	}
	i.factory = fn
	return nil
}

type createTestLogger struct{ runtime.Logger }

func (createTestLogger) Info(string, ...interface{})  {}
func (createTestLogger) Error(string, ...interface{}) {}

func TestInitModuleBlackjackRegistration(t *testing.T) {
	ctx := context.Background()
	initializer := &createTestInitializer{}
	if err := InitModule(ctx, createTestLogger{}, nil, nil, initializer); err != nil {
		t.Fatalf("module registration failed: %v", err)
	}
	wantCalls := []string{"rpc:healthcheck", "rpc:mmad", "match:blackjack", "rpc:blackjack_create"}
	if !slices.Equal(initializer.calls, wantCalls) {
		t.Errorf("registration calls = %v; want %v", initializer.calls, wantCalls)
	}
	if initializer.factory == nil || initializer.rpcs["blackjack_create"] == nil {
		t.Fatal("module must register both the match factory and creation RPC")
	}
	handler, err := initializer.factory(ctx, nil, nil, nil)
	if err != nil || handler == nil {
		t.Fatalf("registered factory should return a handler without an error; got %T, %v", handler, err)
	}
	value, tickRate, label := handler.MatchInit(ctx, nil, nil, nil, map[string]interface{}{"owner_user_id": "owner"})
	state, ok := value.(*BlackjackMatchState)
	if !ok || state == nil || state.ownerUserID != "owner" || tickRate != 5 || label != "blackjack" {
		t.Fatal("registered factory must provide the blackjack handler with its expected initialization behavior")
	}
	nk := &createTestNakama{matchID: "test-match-id"}
	authCtx := context.WithValue(ctx, runtime.RUNTIME_CTX_USER_ID, "rpc-owner")
	response, err := initializer.rpcs["blackjack_create"](authCtx, nil, nil, nk, "")
	if err != nil || response == "" || nk.calls != 1 || nk.params["owner_user_id"] != "rpc-owner" {
		t.Fatal("registered creation RPC must create a match for the authenticated owner")
	}
}

func TestInitModuleBlackjackRegistrationFailures(t *testing.T) {
	registrationError := errors.New("registration failed")
	for _, fail := range []string{"match:blackjack", "rpc:blackjack_create"} {
		t.Run(fail, func(t *testing.T) {
			initializer := &createTestInitializer{fail: fail, err: registrationError}
			err := InitModule(context.Background(), createTestLogger{}, nil, nil, initializer)
			if !errors.Is(err, registrationError) {
				t.Errorf("module initialization should propagate the registration error; got %v", err)
			}
			if fail == "match:blackjack" && slices.Contains(initializer.calls, "rpc:blackjack_create") {
				t.Error("module should stop registration after the match factory fails")
			}
		})
	}
}
