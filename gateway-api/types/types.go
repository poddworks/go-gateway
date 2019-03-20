package types

import (
	. "github.com/poddworks/go-gateway/gateway-api/message"
	. "github.com/streadway/amqp"
)

type ExchangeRequest struct {
	// Name and kind of exchange for ExchangeDeclare
	Name string
	Kind string

	// Standard options for Exchange
	Durable    bool
	AutoDelete bool
	Internal   bool
	NoWait     bool

	// Custom headers for exchange adjustment
	Arguments Table // map[string]interface{}
}

type BindRequest struct {
	RoutingKey string

	// Standard options for Binding Option
	NoWait bool

	// Custom headers for binding adjustment
	Arguments Table // map[string]interface{}
}

type QueueRequest struct {
	// Name of queue for QueueDeclare
	Name string

	// Standard options for Queue
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool

	// Custom headers for queue adjustment
	Arguments Table // map[string]interface{}
}

type SubscriptionRequest struct {
	// Queue to subscribe, must provide this value
	Queue *QueueRequest

	// Exchange to declare or initialize
	Exchange *ExchangeRequest

	// Bind Queue -> Exchange by Routing Keys
	Binding []*BindRequest

	// Standard options for subscription
	AutoAck   bool
	Exclusive bool
	NoLocal   bool
	NoWait    bool

	// Custom headers for queue subscription
	Arguments Table // map[string]interface{}
}

type Payload struct {
	Headers         Table
	ContentType     string
	ContentEncoding string
	CorrelationId   string
	ReplyTo         string
	Content         *Message
}

type CommitRequest struct {
	// Queue to send to, exclusive to Exchange + RoutingKey
	Queue *QueueRequest

	// Exchange to publish to, exclusive to Queue
	Exchange   *ExchangeRequest
	RoutingKey string

	// Embedding Payload
	*Payload
}

func (r *CommitRequest) SetQueue(name string) *CommitRequest {
	if r.Queue != nil {
		r.Queue.Name = name
	}
	return r
}

func (r *CommitRequest) SetExchange(exchange string) *CommitRequest {
	if r.Exchange != nil {
		r.Exchange.Name = exchange
	}
	return r
}

func (r *CommitRequest) SetRoutingKey(key string) *CommitRequest {
	if r.Exchange != nil {
		r.RoutingKey = key
	}
	return r
}

func (r *CommitRequest) Headers(val Table) *CommitRequest {
	if r.Payload != nil {
		r.Payload.Headers = val
	}
	return r
}

func (r *CommitRequest) ContentType(val string) *CommitRequest {
	if r.Payload != nil {
		r.Payload.ContentType = val
	}
	return r
}

func (r *CommitRequest) ContentEncoding(val string) *CommitRequest {
	if r.Payload != nil {
		r.Payload.ContentEncoding = val
	}
	return r
}

func (r *CommitRequest) CorrelationId(val string) *CommitRequest {
	if r.Payload != nil {
		r.Payload.CorrelationId = val
	}
	return r
}

func (r *CommitRequest) ReplyTo(val string) *CommitRequest {
	if r.Payload != nil {
		r.Payload.ReplyTo = val
	}
	return r
}
