// Package invitations issues, validates and redeems the invitations that add a
// person to a senior's care circle.
//
// An invitation is a proposal. It grants nothing until it is accepted, and
// acceptance is what creates the care relationship (docs/04).
package invitations

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"strings"
)

// TokenAlphabet is Crockford's base32 alphabet: the digits and the uppercase
// letters, less I, L, O and U. Dropping those four is what makes a code safe to
// read down a phone line — there is no glyph a reader can confuse with 0 or 1,
// and no way to spell an unfortunate word by accident.
const TokenAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// tokenLength is the number of symbols in a raw invitation token.
//
// Twelve symbols over a 32-symbol alphabet is 60 bits. The token is the sole
// bearer of authority to join a care circle and `GET /v1/invitations/{token}`
// is public, so the number that matters is how long an online search takes:
// against 5,000 outstanding invitations at 5,000 guesses per second, a hit is
// expected roughly once per million years. That holds without rate limiting,
// which is the point — the guarantee comes from the token itself rather than
// from defensive infrastructure that has to keep working.
//
// The earlier scheme used 256 bits, which was stronger still but produced a
// 43-character string nobody could read out or type. Twelve symbols keeps the
// margin overwhelming and the code human.
const tokenLength = 12

// Token is a raw invitation token in canonical form: uppercase, ungrouped, and
// drawn entirely from TokenAlphabet. It exists only in memory and in the single
// response that delivers it; the database stores its hash.
type Token string

// NewToken mints a cryptographically random token.
func NewToken() (Token, error) {
	buffer := make([]byte, tokenLength)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate invitation token: %w", err)
	}

	symbols := make([]byte, tokenLength)
	for i, b := range buffer {
		// 256 is an exact multiple of 32, so the remainder is uniform and this
		// costs no entropy — the usual modulo bias does not arise.
		symbols[i] = TokenAlphabet[b%32]
	}
	return Token(symbols), nil
}

// NormaliseToken converts a code as a person supplies it into canonical form.
//
// A code travels through a share sheet, a text message and a keyboard before it
// comes back, so it arrives grouped, lowercased, padded with spaces, or with
// the letters Crockford excluded standing in for the digits they resemble. All
// of those mean the same invitation and all of them are accepted; anything else
// is rejected, so a malformed code never reaches the database.
func NormaliseToken(raw string) (Token, bool) {
	var builder strings.Builder
	builder.Grow(tokenLength)

	for _, symbol := range strings.ToUpper(raw) {
		switch symbol {
		case ' ', '\t', '\n', '\r', '-':
			// Grouping and stray whitespace carry no meaning.
			continue
		case 'I', 'L':
			symbol = '1'
		case 'O':
			symbol = '0'
		}
		if !strings.ContainsRune(TokenAlphabet, symbol) {
			return "", false
		}
		builder.WriteRune(symbol)
	}

	token := Token(builder.String())
	if !token.Valid() {
		return "", false
	}
	return token, true
}

// Hash returns the value stored in the database.
//
// A plain SHA-256 is the correct primitive for a token this random. Password
// hashes such as bcrypt or argon2 exist to slow brute force against low-entropy
// human secrets; applying one here would add cost to every lookup while
// defending against an attack that cannot succeed anyway.
//
// The hash covers the canonical form, so a code grouped or lowercased on its
// way back to us still finds its invitation — pass it through NormaliseToken
// first.
func (t Token) Hash() []byte {
	sum := sha256.Sum256([]byte(t))
	return sum[:]
}

// Valid reports whether the token is well-formed and canonical.
//
// Rejecting malformed tokens before touching the database keeps a stream of
// junk from turning into a stream of queries. This judges the canonical form
// only; a code as a person typed it has to go through NormaliseToken.
func (t Token) Valid() bool {
	if len(t) != tokenLength {
		return false
	}
	for _, symbol := range t {
		if !strings.ContainsRune(TokenAlphabet, symbol) {
			return false
		}
	}
	return true
}

// EqualHash compares two token hashes in constant time.
//
// Lookups are by indexed hash rather than by scan, so this is not on the hot
// path; it exists for the places that compare a candidate against a known
// value, where timing should not leak how much of the hash matched.
func EqualHash(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
