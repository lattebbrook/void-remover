package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestTurnstileVerifierSendsExpectedRequest(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %q, want POST", r.Method)
			}

			if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
				t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", got)
			}

			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}

			if got := r.Form.Get("secret"); got != "test-secret" {
				t.Errorf("secret = %q, want test-secret", got)
			}

			if got := r.Form.Get("response"); got != "test-token" {
				t.Errorf("response = %q, want test-token", got)
			}

			if got := r.Form.Get("idempotency_key"); got == "" {
				t.Error("idempotency_key is empty")
			}

			return jsonHTTPResponse(http.StatusOK, turnstileResponse{
				Success:  true,
				Hostname: "example.com",
				Action:   "upload",
			}), nil
		}),
	}

	verifier := testTurnstileVerifier(client)
	result, err := verifier.verify(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if !result.Success {
		t.Fatal("result.Success = false, want true")
	}
}

func TestRequireTurnstile(t *testing.T) {
	tests := []struct {
		name              string
		token             string
		response          turnstileResponse
		upstreamStatus    int
		wantStatus        int
		wantHandlerCalled bool
	}{
		{
			name:  "valid token",
			token: "valid-token",
			response: turnstileResponse{
				Success:  true,
				Hostname: "example.com",
				Action:   "upload",
			},
			wantStatus:        fiber.StatusNoContent,
			wantHandlerCalled: true,
		},
		{
			name:       "missing token",
			wantStatus: fiber.StatusBadRequest,
		},
		{
			name:  "rejected token",
			token: "rejected-token",
			response: turnstileResponse{
				Success:    false,
				ErrorCodes: []string{"invalid-input-response"},
			},
			wantStatus: fiber.StatusForbidden,
		},
		{
			name:  "hostname mismatch",
			token: "valid-token",
			response: turnstileResponse{
				Success:  true,
				Hostname: "attacker.example",
				Action:   "upload",
			},
			wantStatus: fiber.StatusForbidden,
		},
		{
			name:  "action mismatch",
			token: "valid-token",
			response: turnstileResponse{
				Success:  true,
				Hostname: "example.com",
				Action:   "different-action",
			},
			wantStatus: fiber.StatusForbidden,
		},
		{
			name:           "siteverify unavailable",
			token:          "valid-token",
			upstreamStatus: http.StatusInternalServerError,
			wantStatus:     fiber.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{
				Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
					if tt.upstreamStatus != 0 {
						return jsonHTTPResponse(tt.upstreamStatus, nil), nil
					}

					return jsonHTTPResponse(http.StatusOK, tt.response), nil
				}),
			}

			verifier := testTurnstileVerifier(client)
			app := fiber.New()
			handlerCalled := false
			app.Post("/upload", requireTurnstile(verifier), func(c fiber.Ctx) error {
				handlerCalled = true
				return c.SendStatus(fiber.StatusNoContent)
			})

			form := url.Values{}
			if tt.token != "" {
				form.Set(turnstileTokenField, tt.token)
			}

			req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			if handlerCalled != tt.wantHandlerCalled {
				t.Errorf("handlerCalled = %v, want %v", handlerCalled, tt.wantHandlerCalled)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonHTTPResponse(status int, body any) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}
}

func testTurnstileVerifier(client *http.Client) *turnstileVerifier {
	return &turnstileVerifier{
		client:           client,
		secretKey:        "test-secret",
		expectedHostname: "example.com",
		expectedAction:   "upload",
		siteverifyURL:    "https://siteverify.test",
	}
}
