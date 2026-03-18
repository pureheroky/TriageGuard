package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const ClaimsKey contextKey = "supabase_claims"

const (
	defaultJWKSCacheTTL = 5 * time.Minute
	jwksRequestTimeout  = 5 * time.Second
)

type jwksResponse struct {
	Keys []map[string]any `json:"keys"`
}

type jwtVerifier struct {
	hmacSecret []byte
	jwksURL    string
	httpClient *http.Client

	mu            sync.RWMutex
	cachedKeys    map[string]any
	cacheUntil    time.Time
	cachedJWKSURL string
}

func newJWTVerifier(secret, supabaseURL string) *jwtVerifier {
	var jwksURL string
	if strings.TrimSpace(supabaseURL) != "" {
		jwksURL = strings.TrimSuffix(strings.TrimSpace(supabaseURL), "/") + "/auth/v1/.well-known/jwks.json"
	}
	return &jwtVerifier{
		hmacSecret: []byte(secret),
		jwksURL:    jwksURL,
		httpClient: &http.Client{Timeout: jwksRequestTimeout},
		cachedKeys: map[string]any{},
	}
}

func (v *jwtVerifier) verify(tokenStr string) (jwt.MapClaims, error) {
	parser := jwt.NewParser()
	unverified, _, err := parser.ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("parse token header: %w", err)
	}

	alg := ""
	if unverified != nil && unverified.Method != nil {
		alg = unverified.Method.Alg()
	}

	keyFn := func(token *jwt.Token) (any, error) {
		method := token.Method.Alg()
		switch method {
		case jwt.SigningMethodHS256.Alg():
			if len(v.hmacSecret) == 0 {
				return nil, fmt.Errorf("missing HMAC secret")
			}
			return v.hmacSecret, nil
		case jwt.SigningMethodRS256.Alg(), jwt.SigningMethodRS384.Alg(), jwt.SigningMethodRS512.Alg(),
			jwt.SigningMethodES256.Alg(), jwt.SigningMethodES384.Alg(), jwt.SigningMethodES512.Alg(),
			jwt.SigningMethodEdDSA.Alg():
			return v.publicKeyForToken(token)
		default:
			return nil, fmt.Errorf("unsupported signing method: %s", method)
		}
	}

	validMethods := []string{
		jwt.SigningMethodHS256.Alg(),
		jwt.SigningMethodRS256.Alg(),
		jwt.SigningMethodRS384.Alg(),
		jwt.SigningMethodRS512.Alg(),
		jwt.SigningMethodES256.Alg(),
		jwt.SigningMethodES384.Alg(),
		jwt.SigningMethodES512.Alg(),
		jwt.SigningMethodEdDSA.Alg(),
	}

	parsed, err := jwt.Parse(tokenStr, keyFn, jwt.WithValidMethods(validMethods))
	if err != nil || !parsed.Valid {
		if err == nil {
			err = fmt.Errorf("invalid token")
		}
		return nil, fmt.Errorf("jwt verify failed (alg=%s): %w", alg, err)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}
	return claims, nil
}

func (v *jwtVerifier) publicKeyForToken(token *jwt.Token) (any, error) {
	jwksURL, err := v.resolveJWKSURL(token)
	if err != nil {
		return nil, err
	}

	keys, err := v.getJWKSKeys(jwksURL)
	if err != nil {
		return nil, err
	}

	kid, _ := token.Header["kid"].(string)
	if kid != "" {
		key, ok := keys[kid]
		if !ok {
			return nil, fmt.Errorf("jwks key not found for kid=%s", kid)
		}
		return key, nil
	}

	if len(keys) == 1 {
		for _, key := range keys {
			return key, nil
		}
	}

	return nil, fmt.Errorf("token missing kid and jwks has %d keys", len(keys))
}

func (v *jwtVerifier) resolveJWKSURL(token *jwt.Token) (string, error) {
	if strings.TrimSpace(v.jwksURL) != "" {
		return v.jwksURL, nil
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if iss, _ := claims["iss"].(string); strings.TrimSpace(iss) != "" {
			return issuerJWKSURL(iss), nil
		}
	}
	return "", fmt.Errorf("SUPABASE_URL is required for asymmetric JWT verification (or token must include iss)")
}

func issuerJWKSURL(issuer string) string {
	base := strings.TrimSuffix(strings.TrimSpace(issuer), "/")
	if strings.HasSuffix(base, "/auth/v1") {
		return base + "/.well-known/jwks.json"
	}
	return base + "/auth/v1/.well-known/jwks.json"
}

func (v *jwtVerifier) getJWKSKeys(jwksURL string) (map[string]any, error) {
	now := time.Now()

	v.mu.RLock()
	if now.Before(v.cacheUntil) && len(v.cachedKeys) > 0 && v.cachedJWKSURL == jwksURL {
		out := make(map[string]any, len(v.cachedKeys))
		for k, val := range v.cachedKeys {
			out[k] = val
		}
		v.mu.RUnlock()
		return out, nil
	}
	v.mu.RUnlock()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create jwks request: %w", err)
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks endpoint status: %d", resp.StatusCode)
	}

	var payload jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode jwks payload: %w", err)
	}
	if len(payload.Keys) == 0 {
		return nil, fmt.Errorf("jwks payload has no keys")
	}

	parsed := make(map[string]any, len(payload.Keys))
	for _, key := range payload.Keys {
		kid, _ := key["kid"].(string)
		if strings.TrimSpace(kid) == "" {
			continue
		}
		pub, err := parseJWKPublicKey(key)
		if err != nil {
			continue
		}
		parsed[kid] = pub
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("no usable keys in jwks payload")
	}

	ttl := parseJWKSMaxAge(resp.Header.Get("Cache-Control"))
	if ttl <= 0 {
		ttl = defaultJWKSCacheTTL
	}

	v.mu.Lock()
	v.cachedKeys = parsed
	v.cacheUntil = now.Add(ttl)
	v.cachedJWKSURL = jwksURL
	out := make(map[string]any, len(v.cachedKeys))
	for k, val := range v.cachedKeys {
		out[k] = val
	}
	v.mu.Unlock()

	return out, nil
}

func parseJWKSMaxAge(cacheControl string) time.Duration {
	if strings.TrimSpace(cacheControl) == "" {
		return 0
	}
	parts := strings.Split(cacheControl, ",")
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if !strings.HasPrefix(strings.ToLower(item), "max-age=") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(item), "max-age="))
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	return 0
}

func parseJWKPublicKey(jwk map[string]any) (any, error) {
	kty, _ := jwk["kty"].(string)
	switch kty {
	case "RSA":
		nStr, _ := jwk["n"].(string)
		eStr, _ := jwk["e"].(string)
		nBytes, err := decodeBase64URL(nStr)
		if err != nil {
			return nil, fmt.Errorf("decode rsa n: %w", err)
		}
		eBytes, err := decodeBase64URL(eStr)
		if err != nil {
			return nil, fmt.Errorf("decode rsa e: %w", err)
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 + int(b)
		}
		if e == 0 {
			return nil, fmt.Errorf("invalid rsa exponent")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
	case "EC":
		crv, _ := jwk["crv"].(string)
		xStr, _ := jwk["x"].(string)
		yStr, _ := jwk["y"].(string)
		xBytes, err := decodeBase64URL(xStr)
		if err != nil {
			return nil, fmt.Errorf("decode ec x: %w", err)
		}
		yBytes, err := decodeBase64URL(yStr)
		if err != nil {
			return nil, fmt.Errorf("decode ec y: %w", err)
		}
		var curve elliptic.Curve
		switch crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("unsupported ec curve: %s", crv)
		}
		pub := &ecdsa.PublicKey{
			Curve: curve,
			X:     new(big.Int).SetBytes(xBytes),
			Y:     new(big.Int).SetBytes(yBytes),
		}
		if !curve.IsOnCurve(pub.X, pub.Y) {
			return nil, fmt.Errorf("ec point not on curve")
		}
		return pub, nil
	case "OKP":
		crv, _ := jwk["crv"].(string)
		xStr, _ := jwk["x"].(string)
		if crv != "Ed25519" {
			return nil, fmt.Errorf("unsupported okp curve: %s", crv)
		}
		xBytes, err := decodeBase64URL(xStr)
		if err != nil {
			return nil, fmt.Errorf("decode okp x: %w", err)
		}
		if len(xBytes) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid ed25519 key size")
		}
		return ed25519.PublicKey(xBytes), nil
	default:
		return nil, fmt.Errorf("unsupported jwk kty: %s", kty)
	}
}

func decodeBase64URL(value string) ([]byte, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("empty value")
	}
	out, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func RequireJWT(secret, supabaseURL string) func(http.Handler) http.Handler {
	verifier := newJWTVerifier(secret, supabaseURL)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			if !strings.HasPrefix(authz, "Bearer ") {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(authz, "Bearer ")
			claims, err := verifier.verify(tokenStr)
			if err != nil {
				log.Printf("auth: jwt verification failed: %v", err)
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
