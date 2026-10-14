package token

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt"
)

const (
	jwtAudience   = "aud"
	jwtExpire     = "exp"
	jwtId         = "jti"
	jwtIssueAt    = "iat"
	jwtIssuer     = "iss"
	jwtNotBefore  = "nbf"
	jwtSubject    = "sub"
	Authorization = "Authorization"
)

var (
	ErrTokenNotFound = errors.New("不存在token")
	ErrTokenInvalid  = errors.New("token is invalid")
	ErrClaimsInvalid = errors.New("invalid token claims")
)

type Parse struct {
	AccessSecret string
}

func NewTokenParse(secret string) *Parse {
	return &Parse{AccessSecret: secret}
}

func (p *Parse) Parse(r *http.Request) (jwt.MapClaims, string, error) {
	tokenStr := p.extractTokenFromHeader(r)
	if len(tokenStr) == 0 {
		return nil, tokenStr, ErrTokenNotFound
	}
	return p.ParseToken(tokenStr)
}

func (p *Parse) ParseToken(tokenStr string) (jwt.MapClaims, string, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return []byte(p.AccessSecret), nil
	})
	if err != nil {
		return nil, tokenStr, err
	}
	if !token.Valid {
		return nil, tokenStr, ErrTokenInvalid
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, tokenStr, ErrClaimsInvalid
	}

	if err = claims.Valid(); err != nil {
		return nil, tokenStr, err
	}
	return claims, tokenStr, nil
}

func (p *Parse) ParseWithContext(r *http.Request) (*http.Request, error) {
	claims, tokenStr, err := p.Parse(r)
	if err != nil {
		return r, err
	}

	ctx := r.Context()
	for k, v := range claims {
		switch k {
		case jwtAudience, jwtExpire, jwtId, jwtIssueAt, jwtIssuer, jwtNotBefore, jwtSubject:
			// ignore the standard claims
		default:
			ctx = context.WithValue(ctx, k, v)
		}
	}

	ctx = context.WithValue(ctx, Authorization, tokenStr)

	return r.WithContext(ctx), nil
}

func (p *Parse) extractTokenFromHeader(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	// Token is expected in the format "Bearer <token>"
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return authHeader
	}

	return parts[1]
}

func GetTokenStr(ctx context.Context) string {
	tokenStr, ok := ctx.Value(Authorization).(string)
	if !ok {
		return ""
	}
	return tokenStr
}
