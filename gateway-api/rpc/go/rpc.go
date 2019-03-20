package rpc

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"encoding/json"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	. "github.com/poddworks/go-gateway/gateway-api/constant"
	. "github.com/poddworks/go-gateway/gateway-api/types"
	. "github.com/streadway/amqp"
)

var (
	DefaultMessageTTL = fmt.Sprintf("%d", (30*time.Second)/time.Millisecond)
)

type AmqpClient struct {
	channelCh  chan *Channel
	retryCh    chan bool
	registerCh chan *stateInfoRequest

	prefetchCount int
	prefetchSize  int
	global        bool

	logger *log.Entry
}

type stateInfo struct {
	channel *Channel
	ctx     context.Context
}

type stateInfoRequest struct {
	stateInfoCh chan *stateInfo
	method      string
	tag         string
}

func requestId() string {
	var generateCh = make(chan string)
	go func() {
		for {
			tag, err := uuid.NewRandom()
			if err == nil {
				generateCh <- tag.String()
				break
			}
		}
	}()
	return <-generateCh
}

func doConnect(endpoint string) (<-chan *Channel, <-chan error) {
	var (
		connection *Connection
		channel    *Channel

		err error

		channelCh = make(chan *Channel, 1)
		errorCh   = make(chan error, 1)
	)
	go func() {
		connection, err = Dial(endpoint)
		if err != nil {
			errorCh <- err
			return
		}

		channel, err = connection.Channel()
		if err != nil {
			errorCh <- err
			return
		}

		channelCh <- channel
	}()
	return channelCh, errorCh
}

func Connect(ctx context.Context, client *AmqpClient, endpoint string) (context.Context, context.CancelFunc) {
	var logger = ctx.Value(LoggingCtxString).(*log.Entry)
	wrapped, cancel := context.WithCancel(ctx)
	go func() {
		var (
			channelCh <-chan *Channel
			errorCh   <-chan error
		)
		for {
			select {
			case <-ctx.Done():
				return // termination by cancel or interruption

			case <-client.retryCh:
				channelCh, errorCh = doConnect(endpoint)
			}

			select {
			case <-ctx.Done():
				return // termination by cancel or interruption

			case err := <-errorCh:
				if err != nil {
					logger.WithFields(log.Fields{"error": err}).Error("Connect")
					go func() {
						<-time.After(MaxConnectRetryDelay)
						client.retryCh <- true
						logger.Debug("Connect-retry")
					}()
				}

			case channel := <-channelCh:
				client.channelCh <- channel
			}
		}
	}()
	client.start(wrapped)
	return wrapped, cancel
}

func New(prefetchCount, prefetchSize int, global bool) *AmqpClient {
	return &AmqpClient{
		channelCh:  make(chan *Channel, 1),
		retryCh:    make(chan bool, 1),
		registerCh: make(chan *stateInfoRequest, 1),

		prefetchCount: prefetchCount,
		prefetchSize:  prefetchSize,
		global:        global,
	}
}

func (client *AmqpClient) register(recvr *stateInfoRequest) {
	recvr.method = "POST"
	client.registerCh <- recvr
}

func (client *AmqpClient) unregister(recvr *stateInfoRequest) {
	recvr.method = "DELETE"
	client.registerCh <- recvr
}

func (client *AmqpClient) start(ctx context.Context) {
	client.logger = ctx.Value(LoggingCtxString).(*log.Entry)
	client.retryCh <- true
	go func() {
		var (
			stateInfos = make(map[string]*stateInfoRequest)
			channel    *Channel

			errorCh = make(chan *Error, 1)
		)
		for {
			select {
			case <-ctx.Done():
				return // termination by cancel or interruption

			case err, ok := <-errorCh:
				client.logger.Debug("start-channel-error")
				if !ok {
					errorCh = make(chan *Error, 1)
				}
				if err != nil {
					client.logger.WithFields(log.Fields{"error": err}).Error("start")
					channel = nil
					go func() {
						<-time.After(MaxConnectRetryDelay)
						client.retryCh <- true
						client.logger.Debug("Connect-retry")
					}()
				}

			case recvr := <-client.registerCh:
				client.logger.WithFields(log.Fields{"method": recvr.method, "tag": recvr.tag}).Debug("start-channel-register")
				if recvr.method == "POST" {
					stateInfos[recvr.tag] = recvr
					go func() {
						if channel != nil {
							recvr.stateInfoCh <- &stateInfo{channel, ctx}
						}
					}()
				}
				if recvr.method == "DELETE" {
					delete(stateInfos, recvr.tag)
				}

			case channel = <-client.channelCh:
				// Received new channel from connect establish, broadcast to
				// all stateInfos to this client
				client.logger.Debug("start-channel-acquired")
				channel.NotifyClose(errorCh)
				channel.Qos(client.prefetchCount, client.prefetchSize, client.global)
				go func() {
					for _, recvr := range stateInfos {
						if channel != nil {
							recvr.stateInfoCh <- &stateInfo{channel, ctx}
						}
					}
				}()
			}
		}
	}()
}

func checkCommitRequest(request *CommitRequest) error {
	if request == nil {
		return errors.New("error/invalid-argument-nil-CommitRequest")
	} else {
		if request.Queue == nil && request.Exchange == nil {
			return errors.New("error/invalid-argument-nil-CommitRequest")
		}
	}
	return nil
}

func getCommitDestination(request *CommitRequest) (exchange, key string) {
	if request.Queue != nil {
		exchange = ""
		key = request.Queue.Name
		return
	}
	if request.Exchange != nil {
		exchange = request.Exchange.Name
		key = request.RoutingKey
		return
	}
	return
}

func doCommit(ctx context.Context, channel *Channel) (chan<- *CommitRequest, <-chan error) {
	availableCh, errorCh := make(chan *CommitRequest), make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return // termination by cancel or interruption

			case message := <-availableCh:
				if message.Payload.ContentType != "application/json" {
					errorCh <- nil
					continue
				}

				body, err := json.Marshal(message.Payload.Content)
				if err != nil {
					errorCh <- nil
					continue
				}

				exchange, key := getCommitDestination(message)

				var payload = Publishing{
					Headers:         message.Payload.Headers,
					ContentType:     message.Payload.ContentType,
					ContentEncoding: message.Payload.ContentEncoding,
					CorrelationId:   message.Payload.CorrelationId,
					ReplyTo:         message.Payload.ReplyTo,
					DeliveryMode:    Persistent,
					Expiration:      DefaultMessageTTL,
					Timestamp:       time.Now(),
					Body:            body,
				}

				err = channel.Publish(exchange, key, false, false, payload)
				errorCh <- err

				if err != nil {
					return // terminate commit worker
				}
			}
		}
	}()
	return availableCh, errorCh
}

func (client *AmqpClient) Commit(ctx context.Context) chan<- *CommitRequest {
	publishCh := make(chan *CommitRequest)

	go func() {
		var wait sync.WaitGroup

		wait.Add(runtime.NumCPU())
		for idx := 0; idx < runtime.NumCPU(); idx++ {
			go func() {
				defer wait.Done()

				var (
					request = &stateInfoRequest{
						stateInfoCh: make(chan *stateInfo, 1),
						tag:         requestId(),
					}

					logger = client.logger.WithFields(log.Fields{"tag": request.tag})

					channel *Channel

					pending []*CommitRequest
					buffer  []*CommitRequest
				)

				// Register stateInfo for channel update
				client.register(request)

				// detach registstration on leave
				defer func() {
					client.unregister(request)
				}()

				for {
					for channel == nil {
						select {
						case <-ctx.Done():
							return // termination by cancel or interruption

						case state := <-request.stateInfoCh:
							channel = state.channel

						case message, ok := <-publishCh:
							if message != nil {
								if checkCommitRequest(message) == nil {
									buffer = append(buffer, message)
									logger.WithFields(log.Fields{"buffer-size": len(buffer), "pending-size": len(pending)}).Debug("Commit-buffer-1")
								}
							}
							if !ok {
								publishCh = nil
							}
						}
					}

					availableCh, errorCh := doCommit(ctx, channel)

					var (
						availableCh2 chan<- *CommitRequest

						creq *CommitRequest
					)
					for channel != nil {
						if len(buffer) > 0 {
							availableCh2, creq = availableCh, buffer[0]
						}
						select {
						case <-ctx.Done():
							return // termination by cancel or interruption

						case err := <-errorCh:
							if err != nil {
								buffer, pending, channel = append(pending, buffer...), []*CommitRequest{}, nil
								logger.WithFields(log.Fields{"buffer-size": len(buffer), "pending-size": len(pending), "error": err}).Error("Commit-pubnack")
							} else {
								pending = pending[1:]
								logger.WithFields(log.Fields{"buffer-size": len(buffer), "pending-size": len(pending)}).Debug("Commit-puback")
							}

						case message, ok := <-publishCh:
							if message != nil {
								if checkCommitRequest(message) == nil {
									buffer = append(buffer, message)
									logger.WithFields(log.Fields{"buffer-size": len(buffer), "pending-size": len(pending)}).Debug("Commit-buffer-2")
								}
							}
							if !ok {
								publishCh = nil
							}

						case availableCh2 <- creq:
							availableCh2, pending, buffer = nil, append(pending, buffer[0]), buffer[1:]
							logger.WithFields(log.Fields{"buffer-size": len(buffer), "pending-size": len(pending)}).Debug("Commit-pub")
						}
					}
				}
			}()
		}
		wait.Wait()
	}()

	return publishCh
}

func (client *AmqpClient) Subscribe(ctx context.Context, opts *SubscriptionRequest) *Subscription {
	subscription, errSub := client.Each(ctx, opts)
	return &Subscription{subscription, errSub, nil}
}

func (client *AmqpClient) Each(ctx context.Context, opts *SubscriptionRequest) (<-chan *MessageRpc, <-chan error) {
	subscription, errSub := make(chan *MessageRpc), make(chan error, 1)

	if opts == nil {
		errSub <- errors.New("error/invalid-argument-nil-SubscriptionRequest")
		close(errSub)
		return nil, errSub
	} else {
		if opts.Queue == nil {
			errSub <- errors.New("error/invalid-argument-nil-QueueRequest")
			close(errSub)
			return nil, errSub
		}
	}

	go func() {
		var (
			request = &stateInfoRequest{
				stateInfoCh: make(chan *stateInfo, 1),
				tag:         requestId(),
			}

			errorCh = make(chan error, 1)

			logger = client.logger.WithFields(log.Fields{"tag": request.tag})
		)

		// Register stateInfo for channel update
		client.register(request)

		// detach registstration on leave
		defer func() {
			client.unregister(request)

			// Announce to caller termination status
			close(subscription)
			close(errSub)
		}()

		for {
			select {
			case <-ctx.Done():
				return // termination by cancel or interruption

			case err := <-errorCh:
				logger.Debug("Each-channel-error")
				if err != nil {
					logger.WithFields(log.Fields{"error": err}).Error("Each")
				}

			case state := <-request.stateInfoCh:
				go func() {
					var (
						channel *Channel = state.channel
						queue   Queue
						err     error

						deliverCh <-chan Delivery

						wait sync.WaitGroup
					)

					// The following section should define how to establish
					// consume queue and or exchange, with binding
					//
					if opts.Queue != nil {
						// QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args Table)
						args := opts.Queue
						queue, err = channel.QueueDeclare(args.Name, args.Durable, args.AutoDelete, args.Exclusive, args.NoWait, args.Arguments)
						if err != nil {
							// Channel will be closed.
							errorCh <- err
							return
						}
					}
					if opts.Exchange != nil {
						// ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args Table)
						args := opts.Exchange
						err = channel.ExchangeDeclare(args.Name, args.Kind, args.Durable, args.AutoDelete, args.Internal, args.NoWait, args.Arguments)
						if err != nil {
							// Channel will be closed.
							errorCh <- err
							return
						}
						if opts.Binding != nil && len(opts.Binding) != 0 {
							for _, binding := range opts.Binding {
								err = channel.QueueBind(queue.Name, binding.RoutingKey, opts.Exchange.Name, binding.NoWait, binding.Arguments)
								if err != nil {
									// Channel will be closed.
									errorCh <- err
									return
								}
							}
						}
					}

					// Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args Table)
					deliverCh, err = channel.Consume(queue.Name, request.tag, opts.AutoAck, opts.Exclusive, opts.NoLocal, opts.NoWait, opts.Arguments)
					if err != nil {
						// Channel will be closed.
						errorCh <- err
						return
					}

					wait.Add(runtime.NumCPU())
					for idx := 0; idx < runtime.NumCPU(); idx++ {
						go func() {
							defer wait.Done()
							for {
								select {
								case <-ctx.Done():
									return // termination by cancel or interruption

								case <-state.ctx.Done():
									return // termination by cancel or interruption

								case delivery, ok := <-deliverCh:
									if !ok {
										return // channel had been terminated
									}
									subscription <- &MessageRpc{
										Payload: &Payload{
											Headers:         delivery.Headers,
											ContentType:     delivery.ContentType,
											ContentEncoding: delivery.ContentEncoding,
											CorrelationId:   delivery.CorrelationId,
											ReplyTo:         delivery.ReplyTo,
											Content:         nil,
										},

										acknowledger: delivery.Acknowledger,
										tag:          delivery.DeliveryTag,
									}
								}
							}
						}()
					}
					wait.Wait()
				}()
			}
		}
	}()

	return subscription, errSub
}
