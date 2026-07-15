package rewards

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const maxRawCodeBytes = 128

const (
	generatedCodeEntropyBytes = 10
	generatedCodeAlphabet     = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

var generatedCodeEncoding = base32.NewEncoding(generatedCodeAlphabet).WithPadding(base32.NoPadding)

var (
	codeVersionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)
	publicCodePattern  = regexp.MustCompile(`^[A-Z0-9]{8,32}$`)
)

type CodeKey struct {
	Version string
	Secret  []byte
}

type CodeKeyring struct {
	current  CodeKey
	previous *CodeKey
}

type codeDigest struct {
	version string
	digest  []byte
}

type encodedKeyring struct {
	Current  encodedCodeKey  `json:"current"`
	Previous *encodedCodeKey `json:"previous,omitempty"`
}

type encodedCodeKey struct {
	Version string `json:"version"`
	Key     string `json:"key"`
}

func ParseCodeKeyring(raw string) (CodeKeyring, error) {
	var encoded encodedKeyring
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encoded); err != nil {
		return CodeKeyring{}, fmt.Errorf("decode REWARDS_CODE_HMAC_KEYS: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
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

func normalizeCode(raw string) (string, error) {
	if len(raw) > maxRawCodeBytes {
		return "", ErrInvalidCommand
	}
	normalized := strings.ToUpper(norm.NFKC.String(strings.TrimSpace(raw)))
	normalized = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.Is(unicode.Hyphen, r) || unicode.Is(unicode.Dash, r) {
			return -1
		}
		return r
	}, normalized)
	if !publicCodePattern.MatchString(normalized) {
		return "", ErrInvalidCommand
	}
	return normalized, nil
}

func (k CodeKeyring) issueDigest(normalized string) codeDigest {
	return codeDigest{version: k.current.Version, digest: calculateCodeDigest(k.current.Secret, normalized)}
}

func (k CodeKeyring) lookupDigests(normalized string) []codeDigest {
	candidates := []codeDigest{k.issueDigest(normalized)}
	if k.previous != nil {
		candidates = append(candidates, codeDigest{
			version: k.previous.Version,
			digest:  calculateCodeDigest(k.previous.Secret, normalized),
		})
	}
	return candidates
}

func calculateCodeDigest(secret []byte, normalized string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(normalized))
	return mac.Sum(nil)
}

func generateSingleUseCode(random io.Reader) (string, error) {
	payload := make([]byte, generatedCodeEntropyBytes)
	if _, err := io.ReadFull(random, payload); err != nil {
		return "", fmt.Errorf("read reward Code entropy: %w", err)
	}
	encoded := generatedCodeEncoding.EncodeToString(payload)
	return encoded[0:4] + "-" + encoded[4:8] + "-" + encoded[8:12] + "-" + encoded[12:16], nil
}
