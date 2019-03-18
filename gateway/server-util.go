package gateway

import (
	"net/http"

	"github.com/google/uuid"
)

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

func notYetImpl(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "error/not-yet-implemented", 400)
}

func notSupported(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "error/not-supported", 400)
}
