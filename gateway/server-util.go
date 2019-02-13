package gateway

import (
	"net/http"
)

func notYetImpl(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "error/not-yet-implemented", 400)
}

func notSupported(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "error/not-supported", 400)
}
