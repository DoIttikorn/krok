package httpx

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// Log logs every request through log and turns a panic into a 500 response,
// so all frameworks log requests the same way. 5xx responses are logged at
// error level, everything else at info.
func Log(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &recorder{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler {
						panic(v) // the server's signal to abort the response
					}
					log.ErrorContext(r.Context(), "panic", "error", v, "stack", string(debug.Stack()))
					if !rec.wroteHeader {
						WriteProblem(rec, http.StatusInternalServerError, "")
					}
				}
				level := slog.LevelInfo
				if rec.status >= http.StatusInternalServerError {
					level = slog.LevelError
				}
				log.LogAttrs(r.Context(), level, "request",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", rec.status),
					slog.Int("bytes", rec.bytes),
					slog.Duration("duration", time.Since(start)),
					slog.String("remote", r.RemoteAddr),
				)
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// recorder remembers the status and size of a response.
type recorder struct {
	http.ResponseWriter
	status, bytes int
	wroteHeader   bool
}

func (r *recorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status, r.wroteHeader = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer, e.g. to
// flush.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
