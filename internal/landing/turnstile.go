package landing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

var errCaptchaMissing = errors.New("captcha: token vacío")

// verifyTurnstile valida el token del widget de Cloudflare. Si no hay secret
// configurado la comprobación se omite (el widget sigue mostrándose).
func (h *Handler) verifyTurnstile(ctx context.Context, token, ip string) error {
	secret := strings.TrimSpace(h.opts.TurnstileSecret)
	if secret == "" {
		return nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errCaptchaMissing
	}

	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)
	if ip != "" {
		form.Set("remoteip", ip)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("captcha: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("captcha: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return fmt.Errorf("captcha: leer respuesta: %w", err)
	}

	var out struct {
		Success    bool     `json:"success"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("captcha: respuesta inválida: %w", err)
	}
	if !out.Success {
		if len(out.ErrorCodes) == 0 {
			return errors.New("captcha: token rechazado")
		}
		return fmt.Errorf("captcha: token rechazado (%s)", strings.Join(out.ErrorCodes, ", "))
	}
	return nil
}
