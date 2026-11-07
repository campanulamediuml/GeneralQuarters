package opponent

import (
	"context"

	"generalquarters/internal/game"
)

type Message struct {
	Author string `json:"author"`
	Text   string `json:"text"`
}

type Plan struct {
	Orders []game.Order `json:"orders"`
	Text   string       `json:"text"`
}

type TextOpponent interface {
	PlanTurn(context.Context, any, []Message) (Plan, error)
}
