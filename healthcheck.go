package main

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/heroiclabs/nakama-common/runtime"
)

type HealthResponse struct {
	Status string `json:"status"`
}

func RpcHealthCheck(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
	payload string,
) (string, error) {
	response := HealthResponse{
		Status: "ok",
	}

	data, err := json.Marshal(response)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
