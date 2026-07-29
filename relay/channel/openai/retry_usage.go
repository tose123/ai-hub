package openai

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

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
	return usage != nil && usage.CompletionTokens > 0
}

func newMissingTextUsageError(c *gin.Context, streamStarted bool) *types.NewAPIError {
	service.InvalidateCurrentChannelAffinityCache(c)
	message := "上游没有返回计费信息，无法扣费（可能是上游超时）"
	if streamStarted {
		message = "Retryable error | please retry later | try again later | rate limit exceeded | temporarily overloaded | Upstream returned no valid billing information | Selected model is at capacity. Please try a different model."
	}
	return types.NewErrorWithStatusCode(
		errors.New(message),
		types.ErrorCodeBadResponse,
		http.StatusInternalServerError,
	)
}

func chatStreamHasSubstantiveOutput(chunk *dto.ChatCompletionsStreamResponse) bool {
	if chunk == nil {
		return false
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.GetContentString() != "" || choice.Delta.GetReasoningContent() != "" ||
			(choice.Delta.Refusal != nil && *choice.Delta.Refusal != "") {
			return true
		}
		if choice.Delta.FunctionCall != nil &&
			(choice.Delta.FunctionCall.Name != "" || choice.Delta.FunctionCall.Arguments != "") {
			return true
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			if toolCall.Function.Name != "" || toolCall.Function.Arguments != "" {
				return true
			}
		}
	}
	return false
}

func responsesStreamHasSubstantiveOutput(event *dto.ResponsesStreamResponse) bool {
	if event == nil {
		return false
	}
	switch event.Type {
	case "response.created", "response.in_progress", "response.queued",
		"response.completed", "response.done", "response.failed", "response.incomplete",
		"response.cancelled", "response.canceled", "response.error":
		return false
	case "response.output_text.delta", "response.reasoning_summary_text.delta",
		"response.reasoning_text.delta", "response.function_call_arguments.delta",
		"response.custom_tool_call_input.delta":
		return event.Delta != ""
	case dto.ResponsesOutputTypeItemAdded, dto.ResponsesOutputTypeItemDone:
		return responsesOutputHasSubstantiveOutput(event.Item)
	case "response.content_part.added", "response.content_part.done",
		"response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		return event.Part != nil && event.Part.Text != ""
	default:
		return true
	}
}

func responsesOutputHasSubstantiveOutput(output *dto.ResponsesOutput) bool {
	if output == nil {
		return false
	}
	if output.Name != "" || output.ArgumentsString() != "" || output.Result != "" {
		return true
	}
	for _, content := range output.Content {
		if content.Text != "" || content.Refusal != "" {
			return true
		}
	}
	for _, summary := range output.Summary {
		if summary.Text != "" {
			return true
		}
	}
	return false
}

func nonRetryableChatStreamReason(choices []dto.ChatCompletionsStreamResponseChoice) string {
	for _, choice := range choices {
		if choice.Delta.Refusal != nil && strings.TrimSpace(*choice.Delta.Refusal) != "" {
			return "content_policy_refusal"
		}
		if choice.FinishReason == nil {
			continue
		}
		reason := strings.ToLower(strings.TrimSpace(*choice.FinishReason))
		if reason == "content_filter" || reason == "length" || reason == "max_output_tokens" {
			return reason
		}
	}
	return ""
}

func nonRetryableTextResponseReason(choices []dto.OpenAITextResponseChoice) string {
	for _, choice := range choices {
		if choice.Message.Refusal != nil && strings.TrimSpace(*choice.Message.Refusal) != "" {
			return "content_policy_refusal"
		}
		reason := strings.ToLower(strings.TrimSpace(choice.FinishReason))
		if reason == "content_filter" || reason == "length" || reason == "max_output_tokens" {
			return reason
		}
	}
	return ""
}

func nonRetryableResponsesError(response *dto.OpenAIResponsesResponse) *types.NewAPIError {
	if response == nil {
		return nil
	}
	if oaiErr := response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
		errorType := strings.ToLower(strings.TrimSpace(oaiErr.Type))
		code := strings.ToLower(strings.TrimSpace(fmt.Sprint(oaiErr.Code)))
		if errorType == "invalid_request_error" || code == "content_filter" || code == "content_policy_violation" {
			return types.WithOpenAIError(*oaiErr, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}
	for _, output := range response.Output {
		for _, content := range output.Content {
			if strings.TrimSpace(content.Refusal) != "" {
				return types.NewOpenAIError(
					errors.New("response refused by content policy"),
					types.ErrorCodeBadResponse,
					http.StatusBadRequest,
					types.ErrOptionWithSkipRetry(),
				)
			}
		}
	}
	if response.IncompleteDetails == nil {
		return nil
	}
	reason := strings.ToLower(strings.TrimSpace(response.IncompleteDetails.Reason))
	if reason != "content_filter" && reason != "max_output_tokens" {
		return nil
	}
	return types.NewOpenAIError(
		fmt.Errorf("responses stream ended with %s", reason),
		types.ErrorCodeBadResponse,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)
}

func newNonRetryableChatFinishError(reason string) *types.NewAPIError {
	return types.NewOpenAIError(
		fmt.Errorf("chat completion ended with %s", reason),
		types.ErrorCodeBadResponse,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
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
