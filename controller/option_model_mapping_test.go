package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionRejectsInvalidGlobalModelMapping(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "invalid JSON", value: "{"},
		{name: "array", value: "[]"},
		{name: "non-string target", value: `{"alias-model":1}`},
		{name: "null", value: "null"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := common.Marshal(OptionUpdateRequest{
				Key:   "global.model_mapping",
				Value: test.value,
			})
			require.NoError(t, err)

			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/", bytes.NewReader(body))

			UpdateOption(ctx)

			require.Equal(t, http.StatusBadRequest, response.Code)
			assert.JSONEq(t, `{"success":false,"message":"Invalid model mapping format"}`, response.Body.String())
		})
	}
}
