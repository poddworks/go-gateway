package rpc

import (
	. "github.com/poddworks/go-gateway/gateway-api/types"
	. "github.com/streadway/amqp"
)

type MessageRpc struct {
	acknowledger Acknowledger
	tag          uint64

	// Embedding Payload
	*Payload
}

func (m *MessageRpc) Ack(multiple bool) error {
	return m.acknowledger.Ack(m.tag, multiple)
}

func (m *MessageRpc) Nack(multiple, requeue bool) error {
	return m.acknowledger.Nack(m.tag, multiple, requeue)
}

func (m *MessageRpc) Reject(requeue bool) error {
	return m.acknowledger.Reject(m.tag, requeue)
}

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
