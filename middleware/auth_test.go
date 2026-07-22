package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

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
