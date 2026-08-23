package middleware

import (
	"log/slog"
	"net/http"
)

// serverErrorRecorder captures server-side failure details for logging.
type serverErrorRecorder interface {
	RecordServerError(err error, publicMessage string)
	StatusCode() int
	ErrorRecorded() (err error, publicMessage string, ok bool)
}

type statusWriter struct {
	http.ResponseWriter
	status        int
	headerWritten bool
	err           error
	publicMessage string
	recorded      bool
}

func (w *statusWriter) WriteHeader(status int) {
	if !w.headerWritten {
		w.status = status
		w.headerWritten = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.status = http.StatusOK
		w.headerWritten = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) RecordServerError(err error, publicMessage string) {
	if err == nil {
		return
	}
	w.err = err
	w.publicMessage = publicMessage
	w.recorded = true
}

func (w *statusWriter) StatusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *statusWriter) ErrorRecorded() (error, string, bool) {
	if !w.recorded {
		return nil, "", false
	}
	return w.err, w.publicMessage, true
}

// ServerErrors logs 5xx responses with request metadata (and error when recorded).
func ServerErrors(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			if rec.StatusCode() < http.StatusInternalServerError {
				return
			}
			attrs := []any{
				"status", rec.StatusCode(),
				"method", r.Method,
				"path", r.URL.Path,
				"request_id", GetRequestID(r.Context()),
			}
			if err, msg, ok := rec.ErrorRecorded(); ok {
				attrs = append(attrs, "error", err.Error(), "message", msg)
				log.ErrorContext(r.Context(), "http_handler_failed", attrs...)
				return
			}
			log.ErrorContext(r.Context(), "http_server_error", attrs...)
		})
	}
}
