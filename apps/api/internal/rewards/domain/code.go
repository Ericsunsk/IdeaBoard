package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	MaxRawCodeBytes       = 128
	MaxCreditAmount int64 = 9_000_000_000_000_000
)

func ValidateCreditAmount(value int64) error {
	if value <= 0 || value > MaxCreditAmount {
		return fmt.Errorf("credit amount must be between 1 and %d", MaxCreditAmount)
	}
	return nil
}

// DeriveAdvisoryLockKey derives a deterministic 64-bit advisory lock key from scope and value.
func DeriveAdvisoryLockKey(scope, value string) int64 {
	digest := sha256.Sum256([]byte(scope + "\x00" + value))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}

var (
	codeVersionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)
	publicCodePattern  = regexp.MustCompile(`^[A-Z0-9]{8,32}$`)
)

// CodeKey is a single HMAC key used for reward code digest issuance or lookup.
type CodeKey struct {
	Version string
	Secret  []byte
}

// CodeKeyring holds the current and optional previous HMAC key versions.
type CodeKeyring struct {
	current  CodeKey
	previous *CodeKey
}

// CodeDigest is the HMAC-SHA256 digest of a normalized code bound to a key version.
type CodeDigest struct {
	Version string
	Digest  []byte
}

type encodedKeyring struct {
	Current  encodedCodeKey  `json:"current"`
	Previous *encodedCodeKey `json:"previous,omitempty"`
}

type encodedCodeKey struct {
	Version string `json:"version"`
	Key     string `json:"key"`
}

// ParseCodeKeyring decodes the REWARDS_CODE_HMAC_KEYS configuration into a validated keyring.
func ParseCodeKeyring(raw string) (CodeKeyring, error) {
	var encoded encodedKeyring
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encoded); err != nil {
		return CodeKeyring{}, fmt.Errorf("decode REWARDS_CODE_HMAC_KEYS: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return CodeKeyring{}, fmt.Errorf("decode REWARDS_CODE_HMAC_KEYS: trailing content: %w", err)
	}
	current, err := decodeCodeKey(encoded.Current)
	if err != nil {
		return CodeKeyring{}, fmt.Errorf("decode current reward code key: %w", err)
	}
	var previous *CodeKey
	if encoded.Previous != nil {
		decoded, err := decodeCodeKey(*encoded.Previous)
		if err != nil {
			return CodeKeyring{}, fmt.Errorf("decode previous reward code key: %w", err)
		}
		previous = &decoded
	}
	return NewCodeKeyring(current, previous)
}

func decodeCodeKey(encoded encodedCodeKey) (CodeKey, error) {
	secret, err := base64.StdEncoding.DecodeString(encoded.Key)
	if err != nil {
		return CodeKey{}, err
	}
	return CodeKey{Version: encoded.Version, Secret: secret}, nil
}

// NewCodeKeyring validates and returns a CodeKeyring, failing when the current key
// is missing or the previous key version collides with the current one.
func NewCodeKeyring(current CodeKey, previous *CodeKey) (CodeKeyring, error) {
	validatedCurrent, err := validateCodeKey(current)
	if err != nil {
		return CodeKeyring{}, fmt.Errorf("current reward code key: %w", err)
	}
	keyring := CodeKeyring{current: validatedCurrent}
	if previous != nil {
		validatedPrevious, err := validateCodeKey(*previous)
		if err != nil {
			return CodeKeyring{}, fmt.Errorf("previous reward code key: %w", err)
		}
		if validatedPrevious.Version == validatedCurrent.Version {
			return CodeKeyring{}, fmt.Errorf("reward code key versions must differ")
		}
		keyring.previous = &validatedPrevious
	}
	return keyring, nil
}

func validateCodeKey(key CodeKey) (CodeKey, error) {
	key.Version = strings.TrimSpace(key.Version)
	if !codeVersionPattern.MatchString(key.Version) || len(key.Secret) < 32 {
		return CodeKey{}, fmt.Errorf("version must be valid and secret must contain at least 32 bytes")
	}
	key.Secret = append([]byte(nil), key.Secret...)
	return key, nil
}

// NormalizeCode applies raw bound, NFKC, uppercase, and Unicode whitespace/hyphen removal.
// It enforces that the final payload is 8–32 characters of [A-Z0-9].
func NormalizeCode(raw string) (string, error) {
	if len(raw) > MaxRawCodeBytes {
		return "", ErrInvalidCode
	}
	normalized := strings.ToUpper(norm.NFKC.String(strings.TrimSpace(raw)))
	normalized = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.Is(unicode.Hyphen, r) || unicode.Is(unicode.Dash, r) {
			return -1
		}
		return r
	}, normalized)
	if !publicCodePattern.MatchString(normalized) {
		return "", ErrInvalidCode
	}
	return normalized, nil
}

// IssueDigest creates a digest using the current key version.
func (k CodeKeyring) IssueDigest(normalized string) CodeDigest {
	return CodeDigest{Version: k.current.Version, Digest: calculateCodeDigest(k.current.Secret, normalized)}
}

// LookupDigests returns digest candidates for current and previous keys,
// so rotation overlap still resolves persisted codes.
func (k CodeKeyring) LookupDigests(normalized string) []CodeDigest {
	candidates := []CodeDigest{k.IssueDigest(normalized)}
	if k.previous != nil {
		candidates = append(candidates, CodeDigest{
			Version: k.previous.Version,
			Digest:  calculateCodeDigest(k.previous.Secret, normalized),
		})
	}
	return candidates
}

// CurrentVersion returns the current key version, useful for persistence.
func (k CodeKeyring) CurrentVersion() string {
	return k.current.Version
}

// PreviousVersion returns the previous key version, or empty string if absent.
func (k CodeKeyring) PreviousVersion() string {
	if k.previous == nil {
		return ""
	}
	return k.previous.Version
}

func calculateCodeDigest(secret []byte, normalized string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(normalized))
	return mac.Sum(nil)
}
