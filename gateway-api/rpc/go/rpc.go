package rpc

import (
	"context"
	"runtime"
	"sync"

	"github.com/google/uuid"
	. "github.com/poddworks/go-gateway/gateway-api/message"
	. "github.com/streadway/amqp"
)

type AmqpClient struct {
	channelCh  chan *Channel
	retryCh    chan bool
	registerCh chan *stateInfoRequest
}

type MessageRpc struct {
	acknowledger Acknowledger

	Content *Message
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

func doConnect(endpoint string) (<-chan *Channel, <-chan error) {
	var (
		connection *Connection
		channel    *Channel

		err error

		channelCh = make(chan *Channel)
		errorCh   = make(chan error)
	)
	go func() {
		connection, err = Dial(endpoint)
		if err != nil {
			// TODO: report error status with logger
			errorCh <- err
		}

		channel, err = connection.Channel()
		if err != nil {
			// TODO: report error status with logger
			errorCh <- err
		}

		channelCh <- channel
	}()
	return channelCh, errorCh
}

func Connect(ctx context.Context, client *AmqpClient, endpoint string) (context.Context, context.CancelFunc) {
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
					// TODO: report error status with logger
					client.retryCh <- true
				}

			case channel := <-channelCh:
				client.channelCh <- channel
			}
		}
	}()
	client.start(wrapped)
	return wrapped, cancel
}

func New() *AmqpClient {
	return &AmqpClient{
		channelCh:  make(chan *Channel),
		retryCh:    make(chan bool),
		registerCh: make(chan *stateInfoRequest),
	}
}

func (client *AmqpClient) start(ctx context.Context) {
	client.retryCh <- true
	go func() {
		var (
			stateInfos map[string]*stateInfoRequest
			channel    *Channel

			errorCh = make(chan *Error)
		)
		for {
			select {
			case <-ctx.Done():
				return // termination by cancel or interruption

			case err := <-errorCh:
				if err != nil {
					// TODO: report error status with logger
					channel = nil
					client.retryCh <- true
				}

			case recvr := <-client.registerCh:
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
				channel.NotifyClose(errorCh)
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

func (client *AmqpClient) register(recvr *stateInfoRequest) {
	recvr.method = "POST"
	client.registerCh <- recvr
}

func (client *AmqpClient) unregister(recvr *stateInfoRequest) {
	recvr.method = "DELETE"
	client.registerCh <- recvr
}

func (client *AmqpClient) Commit() {
	return
}

func (client *AmqpClient) Each(ctx context.Context) <-chan *MessageRpc {
	var subscription = make(chan *MessageRpc)

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

	go func() {
		var (
			request = &stateInfoRequest{
				stateInfoCh: make(chan *stateInfo),
				tag:         <-generateCh,
			}

			errorCh = make(chan error)
		)

		// Register stateInfo for channel update
		client.register(request)

		// detach registstration on leave
		defer func() {
			client.unregister(request)

			// Announce to caller termination status
			close(subscription)
		}()

		for {
			select {
			case <-ctx.Done():
				return // termination by cancel or interruption

			case err := <-errorCh:
				if err != nil {
					// TODO: report error status with logger
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
					// TODO: Implement logic for consume
					//

					// QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args Table)
					queue, err = channel.QueueDeclare("", true, false, false, true, nil)
					if err != nil {
						// Channel will be closed.
						errorCh <- err
						return
					}

					// Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args Table)
					deliverCh, err = channel.Consume(queue.Name, request.tag, false, false, false, false, nil)
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
										acknowledger: delivery.Acknowledger,
										Content:      nil,
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

	return subscription
}
