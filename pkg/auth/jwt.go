package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

// TokenType marks what a token may be used for. Without it, an access token
// could be presented to /auth/refresh to mint fresh token pairs indefinitely,
// which defeats the point of a short access-token TTL.
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
	// TokenTypeTwoFactor 是登录第一步（密码通过）之后、第二步（TOTP 验证码）之前
	// 用的临时凭证。它不能拿去访问任何业务接口，只能用来换取真正的 token 对，
	// 因此即使被截获，攻击者仍然过不了第二因子。
	TokenTypeTwoFactor TokenType = "2fa_pending"
)

// Claims represents JWT claims.
type Claims struct {
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	IsAdmin   bool      `json:"is_admin"`
	TokenType TokenType `json:"token_type"`
	jwt.RegisteredClaims
}

// TokenPair contains access and refresh tokens.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// JWTManager handles JWT token operations.
type JWTManager struct {
	secret            []byte
	accessExpiration  time.Duration
	refreshExpiration time.Duration
}

// NewJWTManager creates a new JWT manager.
func NewJWTManager(secret string, accessExp, refreshExp time.Duration) *JWTManager {
	return &JWTManager{
		secret:            []byte(secret),
		accessExpiration:  accessExp,
		refreshExpiration: refreshExp,
	}
}

// GenerateTokens creates an access and refresh token pair.
func (m *JWTManager) GenerateTokens(userID int64, username string, isAdmin bool) (*TokenPair, error) {
	now := time.Now()

	accessToken, err := m.sign(&Claims{
		UserID:    userID,
		Username:  username,
		IsAdmin:   isAdmin,
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessExpiration)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "stora",
		},
	})
	if err != nil {
		return nil, err
	}

	// The refresh token carries is_admin as well: RefreshTokens re-issues from
	// these claims, so dropping it silently demoted admins on every refresh.
	refreshToken, err := m.sign(&Claims{
		UserID:    userID,
		Username:  username,
		IsAdmin:   isAdmin,
		TokenType: TokenTypeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshExpiration)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "stora",
		},
	})
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "bearer",
		ExpiresIn:    int(m.accessExpiration.Seconds()),
	}, nil
}

// GenerateTwoFactorToken 签发登录第一步通过后的临时凭证。
// 有效期很短：它只是"密码已经对了"的证明，不该在客户端长期留存。
func (m *JWTManager) GenerateTwoFactorToken(userID int64) (string, error) {
	now := time.Now()
	return m.sign(&Claims{
		UserID:    userID,
		TokenType: TokenTypeTwoFactor,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "stora",
		},
	})
}

// ValidateTwoFactorToken 校验临时凭证并返回它指向的用户。
//
// 会拒绝类型不是 2fa_pending 的 token：否则随便拿一个 access token 塞过来，
// 就等于跳过了第二因子。
func (m *JWTManager) ValidateTwoFactorToken(tokenString string) (int64, error) {
	claims, err := m.ValidateToken(tokenString)
	if err != nil {
		return 0, err
	}
	if claims.TokenType != TokenTypeTwoFactor {
		return 0, ErrInvalidToken
	}
	return claims.UserID, nil
}

func (m *JWTManager) sign(claims *Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// ValidateToken validates a token string and returns the claims.
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ValidateAccessToken validates a token and requires it to be an access token.
// Request authentication must use this, so a refresh token cannot be presented
// as a bearer credential.
func (m *JWTManager) ValidateAccessToken(tokenString string) (*Claims, error) {
	claims, err := m.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TokenTypeAccess {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// RefreshTokens generates a new token pair from a valid refresh token.
func (m *JWTManager) RefreshTokens(refreshTokenString string) (*TokenPair, error) {
	claims, err := m.ValidateToken(refreshTokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TokenTypeRefresh {
		return nil, ErrInvalidToken
	}

	return m.GenerateTokens(claims.UserID, claims.Username, claims.IsAdmin)
}
