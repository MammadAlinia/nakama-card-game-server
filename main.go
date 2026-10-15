package main

import (
	"context"
	"database/sql"

	"github.com/heroiclabs/nakama-common/runtime"
)

const rpcHealthCheck = "healthcheck"
const mmad = "mmad"

const matchBlackjack = "blackjack"
const rpcBlackjackCreate = "blackjack_create"

func InitModule(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
	initializer runtime.Initializer,
) error {
	if err := initializer.RegisterRpc(
		rpcHealthCheck,
		RpcHealthCheck,
	); err != nil {
		logger.Error("Failed to register %s RPC: %v", rpcHealthCheck, err)
		return err
	}
	if err := initializer.RegisterRpc(
		mmad,
		MmadRpc,
	); err != nil {
		logger.Error("Failed to register %s RPC: %v", mmad, err)
		return err
	}

	// match
	if err := initializer.RegisterMatch(matchBlackjack, NewBlackjackMatch); err != nil {
		return err
	}
	if err := initializer.RegisterRpc(rpcBlackjackCreate, RpcBlackjackCreate); err != nil {
		return err
	}
	logger.Info("Registered RPC: %s", rpcHealthCheck)
	return nil
}
func NewBlackjackMatch(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
) (runtime.Match, error) {
	// Return a new *BlackjackMatch and nil error.
	return &BlackjackMatch{}, nil
}
