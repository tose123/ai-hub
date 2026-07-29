package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const responsesCompatMessageItemID = "msg_0"

func recordWebSearchCall(info *relaycommon.RelayInfo) {
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return
	}

	for _, toolType := range []string{dto.BuildInToolWebSearch, dto.BuildInToolWebSearchPreview} {
		if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[toolType]; exists && webSearchTool != nil {
			webSearchTool.CallCount++
			return
		}
	}
	info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearch] = &relaycommon.BuildInToolInfo{
		ToolName:          dto.BuildInToolWebSearch,
		CallCount:         1,
		SearchContextSize: "medium",
	}
}

type responsesCompatTextDeltaEvent struct {
	Type         string `json:"type"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

type responsesCompatReasoningDeltaEvent struct {
	Type         string `json:"type"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	SummaryIndex int    `json:"summary_index"`
	Delta        string `json:"delta"`
}

type responsesCompatCompletedEvent struct {
	Type     string                           `json:"type"`
	Response responsesCompatCompletedResponse `json:"response"`
}

type responsesCompatCompletedResponse struct {
	ID        string                  `json:"id"`
	Object    string                  `json:"object"`
	CreatedAt int64                   `json:"created_at"`
	Status    string                  `json:"status"`
	Model     string                  `json:"model"`
	Output    []responsesCompatOutput `json:"output"`
	Usage     *responsesCompatUsage   `json:"usage,omitempty"`
}

type responsesCompatOutput struct {
	Type    string                         `json:"type"`
	ID      string                         `json:"id"`
	Status  string                         `json:"status"`
	Role    string                         `json:"role"`
	Content []responsesCompatOutputContent `json:"content,omitempty"`
}

type responsesCompatOutputContent struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Annotations []any  `json:"annotations"`
}

type responsesCompatUsage struct {
	InputTokens         int                              `json:"input_tokens"`
	OutputTokens        int                              `json:"output_tokens"`
	TotalTokens         int                              `json:"total_tokens"`
	InputTokensDetails  *responsesCompatInputDetails     `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *responsesCompatCompletionDetail `json:"output_tokens_details,omitempty"`
}

type responsesCompatInputDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type responsesCompatCompletionDetail struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	// compute usage
	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
			usage.PromptTokensDetails.CacheWriteTokens = responsesResponse.Usage.InputTokensDetails.CacheWriteTokens
		}
	}
	normalizeTextUsage(&usage)
	usageValid := hasValidTextUsage(&usage)
	if responsesResponse.HasImageGenerationCall() {
		usageValid = service.ValidUsage(&usage)
	}
	if !usageValid {
		if apiErr := nonRetryableResponsesError(&responsesResponse); apiErr != nil {
			return nil, apiErr
		}
		return nil, newMissingTextUsageError(c, false)
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)
	if info == nil {
		return &usage, nil
	}
	if info.ResponsesUsageInfo == nil {
		info.ResponsesUsageInfo = &relaycommon.ResponsesUsageInfo{
			BuiltInTools: make(map[string]*relaycommon.BuildInToolInfo),
		}
	}
	if info.ResponsesUsageInfo.BuiltInTools == nil {
		info.ResponsesUsageInfo.BuiltInTools = make(map[string]*relaycommon.BuildInToolInfo)
	}
	for _, output := range responsesResponse.Output {
		switch output.Type {
		case dto.BuildInCallWebSearchCall:
			recordWebSearchCall(info)
		case dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
		case dto.BuildInCallFunctionCall:
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
		}
	}

	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	if !relaycommon.IsNonBillableResponsesStatus(responsesResponse.Status) {
		for i := range responsesResponse.Output {
			idx := i
			imageCounter.Observe(&responsesResponse.Output[i], &idx)
		}
	}
	imageCounter.Commit(info)

	return &usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	var chatCompatUsed bool
	var responsesCompletedSeen bool
	var chatResponseID string
	var chatCreatedAt int64
	var chatModel string
	var finalCompletedEvent *dto.ResponsesStreamResponse
	var terminalEvent *dto.ResponsesStreamResponse
	var terminalError *types.NewAPIError
	var pendingEvents []struct {
		event dto.ResponsesStreamResponse
		data  string
	}
	var substantiveOutputSent bool
	var hasImageGenerationOutput bool
	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	imageCommitted := false
	flushPendingEvents := func() {
		for _, pending := range pendingEvents {
			sendResponsesStreamData(c, pending.event, pending.data)
		}
		pendingEvents = nil
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		if streamResponse.Type == "" {
			handled, err := handleChatCompletionChunkInResponsesStream(
				c,
				data,
				usage,
				&responseTextBuilder,
				&chatCompatUsed,
				&chatResponseID,
				&chatCreatedAt,
				&chatModel,
			)
			if err != nil {
				logger.LogError(c, "failed to handle chat completion chunk in responses stream: "+err.Error())
				sr.Error(err)
				return
			}
			if handled {
				if common.HasClientVisibleResponse(c) {
					substantiveOutputSent = true
				}
				return
			}
			err = fmt.Errorf("responses stream event missing type")
			logger.LogError(c, err.Error()+": "+data)
			sr.Error(err)
			return
		}
		switch streamResponse.Type {
		case "response.completed", "response.done":
			responsesCompletedSeen = true
			streamCopy := streamResponse
			finalCompletedEvent = &streamCopy
			if streamResponse.Response != nil {
				if streamResponse.Response.HasImageGenerationCall() {
					hasImageGenerationOutput = true
				}
				if streamResponse.Response.Usage != nil {
					if streamResponse.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResponse.Response.Usage.InputTokens
					}
					if streamResponse.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
					}
					if streamResponse.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
					}
					if streamResponse.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
						usage.PromptTokensDetails.CacheWriteTokens = streamResponse.Response.Usage.InputTokensDetails.CacheWriteTokens
					}
				}
				if !imageCommitted {
					if relaycommon.IsNonBillableResponsesStatus(streamResponse.Response.Status) {
						imageCounter.Reset()
						imageCounter.Commit(info)
						imageCommitted = true
					} else {
						for i := range streamResponse.Response.Output {
							idx := i
							imageCounter.Observe(&streamResponse.Response.Output[i], &idx)
						}
						imageCounter.Commit(info)
						imageCommitted = true
					}
				}
			} else if !imageCommitted {
				imageCounter.Commit(info)
				imageCommitted = true
			}
			return
		case "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "response.error":
			streamCopy := streamResponse
			terminalEvent = &streamCopy
			terminalError = nonRetryableResponsesError(streamResponse.Response)
			if terminalError == nil && streamResponse.Response != nil {
				if oaiErr := streamResponse.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					terminalError = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
				}
			}
			if !imageCommitted {
				imageCounter.Reset()
				imageCounter.Commit(info)
				imageCommitted = true
			}
			return
		case "response.output_text.delta":
			// 处理输出文本
			responseTextBuilder.WriteString(streamResponse.Delta)
		case dto.ResponsesOutputTypeItemDone:
			if streamResponse.Item != nil {
				switch streamResponse.Item.Type {
				case dto.BuildInCallWebSearchCall:
					recordWebSearchCall(info)
				case dto.BuildInCallFileSearchCall:
					info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
				case dto.BuildInCallFunctionCall:
					info.CountBillableToolCall(dto.BuildInCallFunctionCall, streamResponse.Item.Name)
				case dto.ResponsesOutputTypeImageGenerationCall:
					hasImageGenerationOutput = true
					if !imageCommitted {
						imageCounter.Observe(streamResponse.Item, streamResponse.OutputIndex)
					}
				}
			}
		}
		if responsesStreamHasSubstantiveOutput(&streamResponse) {
			if !substantiveOutputSent {
				flushPendingEvents()
				substantiveOutputSent = true
			}
			sendResponsesStreamData(c, streamResponse, data)
		} else if substantiveOutputSent {
			sendResponsesStreamData(c, streamResponse, data)
		} else {
			pendingEvents = append(pendingEvents, struct {
				event dto.ResponsesStreamResponse
				data  string
			}{event: streamResponse, data: data})
		}
	})
	if terminalEvent != nil {
		if hasImageGenerationOutput {
			sendResponsesStreamData(c, *terminalEvent, dataFromResponsesEvent(*terminalEvent))
			return usage, nil
		}
		if terminalError != nil && types.IsSkipRetryError(terminalError) {
			if substantiveOutputSent {
				sendResponsesStreamData(c, *terminalEvent, dataFromResponsesEvent(*terminalEvent))
			}
			return nil, terminalError
		}
		if !substantiveOutputSent {
			if terminalError != nil {
				return nil, terminalError
			}
			return nil, newMissingTextUsageError(c, false)
		}
		apiErr := newMissingTextUsageError(c, true)
		responseID := ""
		createdAt := info.StartTime.Unix()
		model := info.UpstreamModelName
		if terminalEvent.Response != nil {
			responseID = terminalEvent.Response.ID
			if terminalEvent.Response.CreatedAt != 0 {
				createdAt = int64(terminalEvent.Response.CreatedAt)
			}
			if terminalEvent.Response.Model != "" {
				model = terminalEvent.Response.Model
			}
		}
		if err := writeResponsesFailedEvent(c, responseID, createdAt, model, apiErr); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		return nil, apiErr
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	normalizeTextUsage(usage)
	usageValid := hasValidTextUsage(usage)
	if hasImageGenerationOutput {
		usageValid = service.ValidUsage(usage)
	}
	if !usageValid {
		if finalCompletedEvent != nil {
			if apiErr := nonRetryableResponsesError(finalCompletedEvent.Response); apiErr != nil {
				if substantiveOutputSent {
					sendResponsesStreamData(c, *finalCompletedEvent, dataFromResponsesEvent(*finalCompletedEvent))
				}
				return nil, apiErr
			}
		}
		if !substantiveOutputSent {
			return nil, newMissingTextUsageError(c, false)
		}
		apiErr := newMissingTextUsageError(c, true)
		if chatCompatUsed {
			if chatResponseID == "" {
				chatResponseID = helper.GetResponseID(c)
			}
			if chatCreatedAt == 0 && !info.StartTime.IsZero() {
				chatCreatedAt = info.StartTime.Unix()
			}
			if chatModel == "" {
				chatModel = info.UpstreamModelName
			}
			if err := writeResponsesFailedEvent(c, chatResponseID, chatCreatedAt, chatModel, apiErr); err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			}
		} else {
			responseID := ""
			createdAt := info.StartTime.Unix()
			model := info.UpstreamModelName
			if finalCompletedEvent != nil && finalCompletedEvent.Response != nil {
				responseID = finalCompletedEvent.Response.ID
				if finalCompletedEvent.Response.CreatedAt != 0 {
					createdAt = int64(finalCompletedEvent.Response.CreatedAt)
				}
				if finalCompletedEvent.Response.Model != "" {
					model = finalCompletedEvent.Response.Model
				}
			}
			if err := writeResponsesFailedEvent(c, responseID, createdAt, model, apiErr); err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			}
		}
		return nil, apiErr
	}
	if !substantiveOutputSent {
		flushPendingEvents()
	}

	if chatCompatUsed && !responsesCompletedSeen {
		if chatResponseID == "" {
			chatResponseID = helper.GetResponseID(c)
		}
		if chatCreatedAt == 0 && !info.StartTime.IsZero() {
			chatCreatedAt = info.StartTime.Unix()
		}
		if chatModel == "" {
			chatModel = info.UpstreamModelName
		}
		if err := sendResponsesCompatCompletedEvent(c, chatResponseID, chatCreatedAt, chatModel, responseTextBuilder.String(), usage); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
		}
	} else if finalCompletedEvent != nil {
		sendResponsesStreamData(c, *finalCompletedEvent, dataFromResponsesEvent(*finalCompletedEvent))
	}

	return usage, nil
}

func dataFromResponsesEvent(event dto.ResponsesStreamResponse) string {
	data, err := common.Marshal(event)
	if err != nil {
		return ""
	}
	return string(data)
}

func handleChatCompletionChunkInResponsesStream(
	c *gin.Context,
	data string,
	usage *dto.Usage,
	responseTextBuilder *strings.Builder,
	chatCompatUsed *bool,
	chatResponseID *string,
	chatCreatedAt *int64,
	chatModel *string,
) (bool, error) {
	var chatChunk dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &chatChunk); err != nil {
		return false, err
	}
	if chatChunk.Object != "chat.completion.chunk" {
		return false, nil
	}

	*chatCompatUsed = true
	if chatChunk.Id != "" {
		*chatResponseID = strings.TrimPrefix(chatChunk.Id, "chatcmpl-")
		if !strings.HasPrefix(*chatResponseID, "resp_") {
			*chatResponseID = "resp_" + *chatResponseID
		}
	}
	if chatChunk.Created != 0 {
		*chatCreatedAt = chatChunk.Created
	}
	if chatChunk.Model != "" {
		*chatModel = chatChunk.Model
	}
	applyChatCompletionsUsageToResponsesUsage(usage, chatChunk.Usage)

	for _, choice := range chatChunk.Choices {
		content := choice.Delta.GetContentString()
		if content != "" {
			responseTextBuilder.WriteString(content)
			if err := sendResponsesCompatTextDelta(c, content); err != nil {
				return true, err
			}
		}
		reasoning := choice.Delta.GetReasoningContent()
		if reasoning != "" {
			responseTextBuilder.WriteString(reasoning)
			if err := sendResponsesCompatReasoningDelta(c, reasoning); err != nil {
				return true, err
			}
		}
	}
	return true, nil
}

func applyChatCompletionsUsageToResponsesUsage(dst *dto.Usage, src *dto.Usage) {
	if dst == nil || src == nil {
		return
	}
	if src.PromptTokens != 0 {
		dst.PromptTokens = src.PromptTokens
		dst.InputTokens = src.PromptTokens
	}
	if src.InputTokens != 0 {
		dst.PromptTokens = src.InputTokens
		dst.InputTokens = src.InputTokens
	}
	if src.CompletionTokens != 0 {
		dst.CompletionTokens = src.CompletionTokens
		dst.OutputTokens = src.CompletionTokens
	}
	if src.OutputTokens != 0 {
		dst.CompletionTokens = src.OutputTokens
		dst.OutputTokens = src.OutputTokens
	}
	if src.TotalTokens != 0 {
		dst.TotalTokens = src.TotalTokens
	}
	if src.PromptTokensDetails.CachedTokens != 0 {
		dst.PromptTokensDetails.CachedTokens = src.PromptTokensDetails.CachedTokens
	}
	if src.InputTokensDetails != nil {
		dst.InputTokensDetails = src.InputTokensDetails
		if src.InputTokensDetails.CachedTokens != 0 {
			dst.PromptTokensDetails.CachedTokens = src.InputTokensDetails.CachedTokens
		}
	}
	if src.CompletionTokenDetails.ReasoningTokens != 0 {
		dst.CompletionTokenDetails.ReasoningTokens = src.CompletionTokenDetails.ReasoningTokens
	}
}

func sendResponsesCompatTextDelta(c *gin.Context, delta string) error {
	event := responsesCompatTextDeltaEvent{
		Type:         "response.output_text.delta",
		ItemID:       responsesCompatMessageItemID,
		OutputIndex:  0,
		ContentIndex: 0,
		Delta:        delta,
	}
	return sendResponsesCompatRawEvent(c, event.Type, event)
}

func sendResponsesCompatReasoningDelta(c *gin.Context, delta string) error {
	event := responsesCompatReasoningDeltaEvent{
		Type:         "response.reasoning_summary_text.delta",
		ItemID:       responsesCompatMessageItemID,
		OutputIndex:  0,
		SummaryIndex: 0,
		Delta:        delta,
	}
	return sendResponsesCompatRawEvent(c, event.Type, event)
}

func sendResponsesCompatCompletedEvent(c *gin.Context, responseID string, createdAt int64, model string, text string, usage *dto.Usage) error {
	content := make([]responsesCompatOutputContent, 0, 1)
	if text != "" {
		content = append(content, responsesCompatOutputContent{
			Type:        "output_text",
			Text:        text,
			Annotations: []any{},
		})
	}
	event := responsesCompatCompletedEvent{
		Type: "response.completed",
		Response: responsesCompatCompletedResponse{
			ID:        responseID,
			Object:    "response",
			CreatedAt: createdAt,
			Status:    "completed",
			Model:     model,
			Output: []responsesCompatOutput{
				{
					Type:    "message",
					ID:      responsesCompatMessageItemID,
					Status:  "completed",
					Role:    "assistant",
					Content: content,
				},
			},
			Usage: responsesCompatUsageFromUsage(usage),
		},
	}
	return sendResponsesCompatRawEvent(c, event.Type, event)
}

func responsesCompatUsageFromUsage(usage *dto.Usage) *responsesCompatUsage {
	if usage == nil {
		return nil
	}
	inputTokens := usage.InputTokens
	if inputTokens == 0 {
		inputTokens = usage.PromptTokens
	}
	outputTokens := usage.OutputTokens
	if outputTokens == 0 {
		outputTokens = usage.CompletionTokens
	}
	totalTokens := usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = inputTokens + outputTokens
	}
	if inputTokens == 0 && outputTokens == 0 && totalTokens == 0 {
		return nil
	}

	compatUsage := &responsesCompatUsage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  totalTokens,
	}
	cachedTokens := usage.PromptTokensDetails.CachedTokens
	if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens != 0 {
		cachedTokens = usage.InputTokensDetails.CachedTokens
	}
	if cachedTokens != 0 {
		compatUsage.InputTokensDetails = &responsesCompatInputDetails{CachedTokens: cachedTokens}
	}
	if usage.CompletionTokenDetails.ReasoningTokens != 0 {
		compatUsage.OutputTokensDetails = &responsesCompatCompletionDetail{ReasoningTokens: usage.CompletionTokenDetails.ReasoningTokens}
	}
	return compatUsage
}

func sendResponsesCompatRawEvent(c *gin.Context, eventType string, event any) error {
	data, err := common.Marshal(event)
	if err != nil {
		return err
	}
	sendResponsesStreamData(c, dto.ResponsesStreamResponse{Type: eventType}, string(data))
	return nil
}
