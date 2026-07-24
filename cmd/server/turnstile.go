package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const (
	turnstileSiteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	turnstileTokenField    = "cf-turnstile-response"
	turnstileMaxTokenSize  = 2048
	turnstileTimeout       = 5 * time.Second
	turnstileResponseLimit = 64 * 1024
)

type turnstileVerifier struct {
	client           *http.Client
	secretKey        string
	expectedHostname string
	expectedAction   string
	siteverifyURL    string
}

type turnstileResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes"`
	Action      string   `json:"action"`
}

func newTurnstileVerifier() (*turnstileVerifier, error) {
	secretKey := strings.TrimSpace(os.Getenv("TURNSTILE_SECRET_KEY"))
	if secretKey == "" {
		return nil, fmt.Errorf("TURNSTILE_SECRET_KEY is required")
	}

	expectedHostname := strings.ToLower(strings.TrimSpace(os.Getenv("TURNSTILE_EXPECTED_HOSTNAME")))
	if expectedHostname == "" {
		return nil, fmt.Errorf("TURNSTILE_EXPECTED_HOSTNAME is required")
	}

	expectedAction := strings.TrimSpace(os.Getenv("TURNSTILE_EXPECTED_ACTION"))

	return &turnstileVerifier{
		client: &http.Client{
			Timeout: turnstileTimeout,
		},
		secretKey:        secretKey,
		expectedHostname: expectedHostname,
		expectedAction:   expectedAction,
		siteverifyURL:    turnstileSiteverifyURL,
	}, nil
}

func (v *turnstileVerifier) verify(ctx context.Context, token string) (turnstileResponse, error) {
	form := url.Values{}
	form.Set("secret", v.secretKey)
	form.Set("response", token)
	form.Set("idempotency_key", uuid.NewString())

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		v.siteverifyURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return turnstileResponse{}, fmt.Errorf("create Siteverify request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		return turnstileResponse{}, fmt.Errorf("call Siteverify: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return turnstileResponse{}, fmt.Errorf("Siteverify returned status %d", resp.StatusCode)
	}

	var result turnstileResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, turnstileResponseLimit)).Decode(&result); err != nil {
		return turnstileResponse{}, fmt.Errorf("decode Siteverify response: %w", err)
	}

	return result, nil
}

func requireTurnstile(verifier *turnstileVerifier) fiber.Handler {
	return func(c fiber.Ctx) error {
		token := strings.TrimSpace(c.FormValue(turnstileTokenField))
		if token == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "TURNSTILE_TOKEN_REQUIRED",
			})
		}

		if len(token) > turnstileMaxTokenSize {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "TURNSTILE_TOKEN_INVALID",
			})
		}

		ctx, cancel := context.WithTimeout(c.Context(), turnstileTimeout)
		defer cancel()

		result, err := verifier.verify(ctx, token)
		if err != nil {
			log.Printf("Turnstile Siteverify request failed: %v", err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "TURNSTILE_UNAVAILABLE",
			})
		}

		if !result.Success {
			log.Printf("Turnstile rejected token: error_codes=%v", result.ErrorCodes)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "TURNSTILE_VERIFICATION_FAILED",
			})
		}

		if !strings.EqualFold(result.Hostname, verifier.expectedHostname) {
			log.Printf(
				"Turnstile hostname mismatch: expected=%q received=%q",
				verifier.expectedHostname,
				result.Hostname,
			)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "TURNSTILE_VERIFICATION_FAILED",
			})
		}

		if verifier.expectedAction != "" && result.Action != verifier.expectedAction {
			log.Printf(
				"Turnstile action mismatch: expected=%q received=%q",
				verifier.expectedAction,
				result.Action,
			)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "TURNSTILE_VERIFICATION_FAILED",
			})
		}

		return c.Next()
	}
}
