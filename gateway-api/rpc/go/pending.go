package rpc

import (
	"sync"

	. "github.com/poddworks/go-gateway/gateway-api/types"
)

type PendingCommitRequest map[string]*CommitRequest

var (
	pendingCommitRequest PendingCommitRequest

	mux sync.Mutex
)

func (p PendingCommitRequest) Get(requestId string) *CommitRequest {
	mux.Lock()
	defer mux.Unlock()
	request, ok := p[requestId]
	if !ok {
		return nil
	}
	return request
}

func (p PendingCommitRequest) Once(requestId string) *CommitRequest {
	mux.Lock()
	defer mux.Unlock()
	request, ok := p[requestId]
	if !ok {
		return nil
	}
	delete(p, requestId)
	return request
}

func (p PendingCommitRequest) Set(request *CommitRequest) {
	mux.Lock()
	defer mux.Unlock()
	var requestId string = request.RequestId()
	p[requestId] = request
}

func init() {
	pendingCommitRequest = make(map[string]*CommitRequest)
}
