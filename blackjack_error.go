package main

import (
	"encoding/json"
	"errors"

	"github.com/heroiclabs/nakama-common/runtime"
)

type CommandErrorResponse struct {
	Command int64  `json:"command"`
	Message string `json:"message"`
}

func (s *BlackjackMatchState) sendError(
	dispatcher runtime.MatchDispatcher,
	command int64,
	message string,
) error {
	if !s.joined || s.playerPresence == nil {
		return errors.New("player is not connected")
	}

	data, err := json.Marshal(CommandErrorResponse{
		Command: command,
		Message: message,
	})
	if err != nil {
		return err
	}

	return dispatcher.BroadcastMessage(
		opError,
		data,
		[]runtime.Presence{s.playerPresence},
		nil,
		true,
	)
}
