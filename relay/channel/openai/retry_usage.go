package openai

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func normalizeTextUsage(usage *dto.Usage) *dto.Usage {
	if usage == nil {
		return nil
	}
	if usage.PromptTokens == 0 && usage.InputTokens != 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.CompletionTokens == 0 && usage.OutputTokens != 0 {
		usage.CompletionTokens = usage.OutputTokens
	}
	if usage.InputTokens == 0 && usage.PromptTokens != 0 {
		usage.InputTokens = usage.PromptTokens
	}
	if usage.OutputTokens == 0 && usage.CompletionTokens != 0 {
		usage.OutputTokens = usage.CompletionTokens
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}

func hasValidTextUsage(usage *dto.Usage) bool {
	usage = normalizeTextUsage(usage)
	return usage != nil && usage.PromptTokens+usage.CompletionTokens > 0
}

func newMissingTextUsageRetryableError(c *gin.Context) *types.NewAPIError {
	service.InvalidateCurrentChannelAffinityCache(c)
	return types.NewErrorWithStatusCode(
		errors.New("Retryable error | please retry later | try again later | rate limit exceeded | temporarily overloaded | Upstream returned no valid billing information | Selected model is at capacity. Please try a different model."),
		types.ErrorCodeBadResponse,
		http.StatusTooManyRequests,
	)
}

func buildStreamOpenAIError(c *gin.Context, apiErr *types.NewAPIError) types.OpenAIError {
	oaiErr := apiErr.ToOpenAIError()
	if c != nil {
		requestID := c.GetString(common.RequestIdKey)
		if requestID != "" {
			oaiErr.Message = common.MessageWithRequestId(oaiErr.Message, requestID)
		}
	}
	return oaiErr
}

func writeChatStreamTerminalError(c *gin.Context, apiErr *types.NewAPIError) error {
	return helper.ObjectData(c, gin.H{
		"error": buildStreamOpenAIError(c, apiErr),
	})
}

type responsesFailedEvent struct {
	Type     string                `json:"type"`
	Response responsesFailedDetail `json:"response"`
}

type responsesFailedDetail struct {
	ID        string            `json:"id"`
	Object    string            `json:"object"`
	CreatedAt int64             `json:"created_at"`
	Status    string            `json:"status"`
	Model     string            `json:"model"`
	Error     types.OpenAIError `json:"error"`
}

func writeResponsesFailedEvent(c *gin.Context, responseID string, createdAt int64, model string, apiErr *types.NewAPIError) error {
	if responseID == "" {
		responseID = helper.GetResponseID(c)
	}
	event := responsesFailedEvent{
		Type: "response.failed",
		Response: responsesFailedDetail{
			ID:        responseID,
			Object:    "response",
			CreatedAt: createdAt,
			Status:    "failed",
			Model:     model,
			Error:     buildStreamOpenAIError(c, apiErr),
		},
	}
	return sendResponsesCompatRawEvent(c, event.Type, event)
}

func cloneUsageForResponses(usage *dto.Usage) *dto.Usage {
	usage = normalizeTextUsage(usage)
	if usage == nil {
		return nil
	}
	cloned := *usage
	if usage.InputTokensDetails != nil {
		details := *usage.InputTokensDetails
		cloned.InputTokensDetails = &details
	} else if usage.PromptTokensDetails.CachedTokens != 0 ||
		usage.PromptTokensDetails.CachedCreationTokens != 0 ||
		usage.PromptTokensDetails.TextTokens != 0 ||
		usage.PromptTokensDetails.AudioTokens != 0 ||
		usage.PromptTokensDetails.ImageTokens != 0 {
		details := usage.PromptTokensDetails
		cloned.InputTokensDetails = &details
	}
	return &cloned
}
