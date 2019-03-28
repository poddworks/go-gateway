package rpc

import (
	. "github.com/poddworks/go-gateway/gateway-api/types"
)

type Subscription struct {
	subscription <-chan *MessageRpc
	errSub       <-chan error

	lastError error
}

func (s *Subscription) Stream() <-chan *MessageRpc {
	go func() {
		if err := <-s.errSub; err != nil {
			s.lastError = err
		}
	}()
	return s.subscription
}

func (s *Subscription) Error() error {
	return s.lastError
}
