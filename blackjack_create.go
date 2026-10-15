package main

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/heroiclabs/nakama-common/runtime"
)

type CreateBlackjackResponse struct {
	MatchId string `json:"match_id"`
}

func RpcBlackjackCreate(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
	payload string,
) (string, error) {
	userID, ok := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	if !ok || userID == "" {
		return "", runtime.NewError("authentication required", 16)
	}
	matchID, err := nk.MatchCreate(
		ctx,
		matchBlackjack,
		map[string]interface{}{"owner_user_id": userID},
	)
	if err != nil {
		return "", err
	}
	if matchID == "" {
		return "", runtime.NewError("match creation failed", 13)
	}
	response, err := json.Marshal(&CreateBlackjackResponse{MatchId: matchID})

	if err != nil {
		return "", err
	}
	return string(response), nil
}
