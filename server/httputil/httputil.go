package httputil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-kit/kit/log"
	httptransport "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
)

func NewRouter(logger log.Logger) (*mux.Router, []httptransport.ServerOption) {
	r := mux.NewRouter()
	options := []httptransport.ServerOption{
		httptransport.ServerErrorEncoder(ErrorEncoder),
		httptransport.ServerErrorLogger(logger),
		httptransport.ServerBefore(httptransport.PopulateRequestContext),
	}
	return r, options
}

func EncodeJSONResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if f, ok := response.(failer); ok && f.Failed() != nil {
		ErrorEncoder(ctx, f.Failed(), w)
		return nil
	}
	if headerer, ok := response.(httptransport.Headerer); ok {
		for k, values := range headerer.Headers() {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	code := http.StatusOK
	if sc, ok := response.(httptransport.StatusCoder); ok {
		code = sc.StatusCode()
	}
	w.WriteHeader(code)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(response)
}

func ErrorEncoder(_ context.Context, err error, w http.ResponseWriter) {
	errMap := map[string]interface{}{"error": err.Error()}

	// pattern match on custom errors
	{
		type authenticationError interface {
			error
			AuthenticationError() string
		}
		if e, ok := err.(authenticationError); ok {
			errMap["error"] = e.AuthenticationError()
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	if headerer, ok := err.(httptransport.Headerer); ok {
		for k, values := range headerer.Headers() {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	code := http.StatusInternalServerError
	if sc, ok := err.(httptransport.StatusCoder); ok {
		code = sc.StatusCode()
	}
	w.WriteHeader(code)

	enc.Encode(errMap)
}

// failer is an interface that should be implemented by response types.
// Response encoders can check if responses are Failer, and if so if they've
// failed, and if so encode them using a separate write path based on the error.
type failer interface {
	Failed() error
}

type errorWrapper struct {
	Error string `json:"error"`
}

func JSONErrorDecoder(r *http.Response) error {
	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return fmt.Errorf("expected JSON formatted error, got Content-Type %s", contentType)
	}
	var w errorWrapper
	if err := json.NewDecoder(r.Body).Decode(&w); err != nil {
		return err
	}
	return errors.New(w.Error)
}

func CopyURL(base *url.URL, path string) *url.URL {
	next := *base
	next.Path = path
	return &next
}

func DecodeJSONRequest(r *http.Request, into interface{}) error {
	err := json.NewDecoder(r.Body).Decode(into)
	return err
}

func DecodeJSONResponse(r *http.Response, into interface{}) error {
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return JSONErrorDecoder(r)
	}

	err := json.NewDecoder(r.Body).Decode(into)
	return err
}
