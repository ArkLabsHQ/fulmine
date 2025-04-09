package envunlocker

import (
	"context"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
)

type service struct {
	password string
}

func NewService(password string) (ports.Unlocker, error) {
	if len(password) <= 0 {
		return nil, fmt.Errorf("missing password in environment")
	}
	return &service{password}, nil
}

func (s *service) GetPassword(_ context.Context) (string, error) {
	return s.password, nil
}
