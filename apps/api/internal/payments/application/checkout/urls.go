package checkout

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	paymentsdomain "ideaboard/api/internal/payments/domain"
)

func NormalizeReturnBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", paymentsdomain.ErrPurchasesUnavailable
	}
	hostname := parsed.Hostname()
	isLoopback := hostname == "localhost" || hostname == "127.0.0.1"
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !isLoopback) {
		return "", paymentsdomain.ErrPurchasesUnavailable
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func ReturnURL(baseURL, publicToken, source string) string {
	return baseURL + "?account=recharge&payment_return=" + url.QueryEscape(publicToken) + "&checkout_return=" + source
}

func NewPublicOrderNumber(now time.Time) (string, error) {
	randomBytes := make([]byte, 6)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate public payment order number: %w", err)
	}
	return "IBP-" + now.Format("20060102") + "-" + strings.ToUpper(hex.EncodeToString(randomBytes)), nil
}
