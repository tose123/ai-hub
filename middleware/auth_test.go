package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDashboardAuthMiddlewareTest(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.SessionSecret = "middleware-auth-test-secret"
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
	})
}

func issueExpiredDashboardAccessToken(t *testing.T, identity service.AuthIdentity) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":       "new-api",
		"aud":       []string{"new-api-dashboard"},
		"sub":       fmt.Sprintf("%d", identity.UserID),
		"token_use": "access",
		"sid":       identity.SessionID,
		"uv":        identity.UserAuthVersion,
		"sv":        identity.SessionVersion,
		"exp":       time.Now().Add(-time.Minute).Unix(),
		"nbf":       time.Now().Add(-2 * time.Minute).Unix(),
		"iat":       time.Now().Add(-2 * time.Minute).Unix(),
	}
	mac := hmac.New(sha256.New, []byte(common.SessionSecret))
	_, err := mac.Write([]byte("new-api/auth/access/v1"))
	require.NoError(t, err)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(mac.Sum(nil))
	require.NoError(t, err)
	return token
}

func TestAnytoolsAuth(t *testing.T) {
	tests := []struct {
		name        string
		serverToken string
		headerToken string
		wantStatus  int
	}{
		{name: "matching token", serverToken: "shared-secret", headerToken: "shared-secret", wantStatus: http.StatusNoContent},
		{name: "missing header", serverToken: "shared-secret", wantStatus: http.StatusUnauthorized},
		{name: "wrong token", serverToken: "shared-secret", headerToken: "wrong-secret", wantStatus: http.StatusUnauthorized},
		{name: "missing configuration", wantStatus: http.StatusServiceUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ANYTOOLS_AUTH_TOKEN", test.serverToken)
			router := gin.New()
			router.Use(AnytoolsAuth())
			router.GET("/", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.headerToken != "" {
				request.Header.Set("X-Anytools-Token", test.headerToken)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			assert.Equal(t, test.wantStatus, recorder.Code)
		})
	}
}
