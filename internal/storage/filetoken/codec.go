// Package filetoken seals a stored file's value, and the holder allowed to use
// it, into an opaque token, and opens such tokens again.
//
// A token is a compact JWE ("dir" + A256GCM). Its claims are encrypted, and
// the GCM tag makes any change to them or to the header detectable, so only
// the deployment holding the keyset can read or forge one. The package does
// not know who a holder is or how long a token should last: callers decide
// both.
package filetoken

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

// typ is the JWE "typ" header every token carries. Open requires it, so a JWE
// made for some other purpose never opens as a file token.
const typ = "nsw-file+jwe"

// maxTokenLength bounds what Open parses. A token carries one stored value
// and one holder, so a real one is well under a kilobyte, even when the value
// is itself a token from another deployment.
const maxTokenLength = 2048

var (
	// ErrInvalid means the token is not one this keyset sealed, or has been
	// altered.
	ErrInvalid = errors.New("filetoken: invalid token")
	// ErrExpired means the token is intact but past its expiry.
	ErrExpired = errors.New("filetoken: token expired")
)

// Claims is what a token carries.
type Claims struct {
	// Value is the stored value the token stands for.
	Value string
	// Holder names who may use the token, in whatever form the caller chose.
	Holder string
	// IssuedAt is when the token was sealed.
	IssuedAt time.Time
	// Expires is when the token stops being usable. It is zero for a token
	// that never expires.
	Expires time.Time
}

// payload is the encrypted JSON form of Claims. Times are Unix seconds.
type payload struct {
	Value    string `json:"v"`
	Holder   string `json:"h"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp,omitempty"`
}

// Codec seals and opens tokens with one keyset.
type Codec struct {
	keys *Keyset
	now  func() time.Time
}

// NewCodec returns a Codec that seals with the keyset's newest key. now is the
// clock tokens are stamped and checked against; nil means time.Now.
func NewCodec(keys *Keyset, now func() time.Time) (*Codec, error) {
	if keys == nil || keys.current == "" {
		return nil, errors.New("filetoken: keyset is empty")
	}
	if now == nil {
		now = time.Now
	}
	return &Codec{keys: keys, now: now}, nil
}

// Seal returns a token carrying value and holder. lifetime is how long the
// token is usable for; zero means it never expires.
func (c *Codec) Seal(value, holder string, lifetime time.Duration) (string, error) {
	if value == "" || holder == "" {
		return "", errors.New("filetoken: value and holder are required")
	}
	if lifetime < 0 {
		return "", errors.New("filetoken: lifetime must not be negative")
	}
	now := c.now()
	p := payload{Value: value, Holder: holder, IssuedAt: now.Unix()}
	if lifetime > 0 {
		p.Expires = now.Add(lifetime).Unix()
	}
	plaintext, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("filetoken: encode claims: %w", err)
	}
	enc, err := jose.NewEncrypter(jose.A256GCM,
		jose.Recipient{Algorithm: jose.DIRECT, Key: c.keys.keys[c.keys.current], KeyID: c.keys.current},
		(&jose.EncrypterOptions{}).WithType(typ))
	if err != nil {
		return "", fmt.Errorf("filetoken: create encrypter: %w", err)
	}
	obj, err := enc.Encrypt(plaintext)
	if err != nil {
		return "", fmt.Errorf("filetoken: encrypt: %w", err)
	}
	token, err := obj.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("filetoken: serialize: %w", err)
	}
	if len(token) > maxTokenLength {
		return "", fmt.Errorf("filetoken: value too long: the token would be %d bytes, Open accepts at most %d", len(token), maxTokenLength)
	}
	return token, nil
}

// Open returns the claims sealed in token. A token that fails any check is
// ErrInvalid. One that is intact but expired is ErrExpired, returned with its
// claims, so a caller can still tell whose it was.
func (c *Codec) Open(token string) (Claims, error) {
	if len(token) > maxTokenLength {
		return Claims{}, ErrInvalid
	}
	obj, err := jose.ParseEncryptedCompact(token, []jose.KeyAlgorithm{jose.DIRECT}, []jose.ContentEncryption{jose.A256GCM})
	if err != nil || obj.Header.ExtraHeaders[jose.HeaderType] != typ {
		return Claims{}, ErrInvalid
	}
	key, ok := c.keys.keys[obj.Header.KeyID]
	if !ok {
		return Claims{}, ErrInvalid
	}
	plaintext, err := obj.Decrypt(key)
	if err != nil {
		return Claims{}, ErrInvalid
	}
	var p payload
	if err := json.Unmarshal(plaintext, &p); err != nil || p.Value == "" || p.Holder == "" {
		return Claims{}, ErrInvalid
	}
	claims := Claims{Value: p.Value, Holder: p.Holder, IssuedAt: time.Unix(p.IssuedAt, 0)}
	if p.Expires != 0 {
		claims.Expires = time.Unix(p.Expires, 0)
		if !c.now().Before(claims.Expires) {
			return claims, ErrExpired
		}
	}
	return claims, nil
}
