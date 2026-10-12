package main

import (
	"context"
	"database/sql"

	"github.com/heroiclabs/nakama-common/runtime"
)

const rpcHealthCheck = "healthcheck"
const mmad = "mmad"

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
	logger.Info("Registered RPC: %s", rpcHealthCheck)
	return nil
}
