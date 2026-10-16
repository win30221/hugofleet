package pricing

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"
)

const MaxBodyBytes = 4096

type Config struct {
	Version string
	Ready   func() bool
	Logger  *slog.Logger
}

type errorResponse struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

func ValidVersion(version string) bool {
	return version == "v1" || version == "v2-good" || version == "v2-broken"
}

func NewHandler(config Config) (http.Handler, error) {
	if !ValidVersion(config.Version) {
		return nil, errors.New("version must be v1, v2-good or v2-broken")
	}
	if config.Ready == nil {
		config.Ready = func() bool { return true }
	}
	if config.Logger == nil {
		config.Logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/quote", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
		defer r.Body.Close()
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var body struct {
			AmountCents  *int64          `json:"amount_cents"`
			CustomerTier string          `json:"customer_tier"`
			CouponCode   json.RawMessage `json:"coupon_code"`
		}
		if err := decoder.Decode(&body); err != nil {
			writeDecodeError(w, err)
			return
		}
		// Reject trailing documents and oversized whitespace, not just oversized JSON values.
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			writeDecodeError(w, err)
			return
		}
		request := QuoteRequest{AmountCents: body.AmountCents, CustomerTier: body.CustomerTier}
		if len(body.CouponCode) > 0 {
			var coupon *string
			if err := json.Unmarshal(body.CouponCode, &coupon); err != nil || coupon == nil {
				writeDecodeError(w, errors.New("coupon_code must be a string"))
				return
			}
			request.CouponCode = *coupon
		}
		quote, err := Calculate(request)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_ARGUMENT", "amount_cents must be an integer from 0 to 1000000000000; customer_tier must be standard or vip; coupon_code must be empty or SAVE100")
			return
		}
		writeJSON(w, http.StatusOK, quote)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if requireMethod(w, r, http.MethodGet) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		}
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if config.Version == "v2-broken" || !config.Ready() {
			writeError(w, http.StatusServiceUnavailable, "NOT_READY", "application is not ready")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		if requireMethod(w, r, http.MethodGet) {
			writeJSON(w, http.StatusOK, map[string]string{"version": config.Version})
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "endpoint not found")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var randomID [12]byte
		if _, err := rand.Read(randomID[:]); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "request ID generation failed")
			return
		}
		requestID := hex.EncodeToString(randomID[:])
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		started := time.Now()
		recorder := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		mux.ServeHTTP(recorder, r)
		config.Logger.Info("http_request", "request_id", requestID, "method", r.Method, "path", r.URL.Path,
			"status", recorder.status, "duration_ms", time.Since(started).Milliseconds())
	}), nil
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "unsupported HTTP method")
	return false
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body exceeds 4096 bytes")
		return
	}
	writeError(w, http.StatusBadRequest, "INVALID_JSON", "body must contain exactly one JSON object with supported fields")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var response errorResponse
	response.Error.Code = code
	response.Error.Message = message
	response.Error.RequestID = w.Header().Get("X-Request-ID")
	writeJSON(w, status, response)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// All response types are JSON-safe. Connection write errors cannot be recovered here.
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Warn("response_write_failed", "request_id", w.Header().Get("X-Request-ID"), "error", err.Error())
		return
	}
}
