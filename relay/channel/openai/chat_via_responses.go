package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var responsesResp dto.OpenAIResponsesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if err := common.Unmarshal(body, &responsesResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := responsesResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	chatResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, &responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	chatResp, ok := chatResult.Value.(*dto.OpenAITextResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI chat response, got %T", chatResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if chatID := helper.GetResponseID(c); chatID != "" {
		chatResp.Id = chatID
	}
	usage := chatResult.Usage

	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(&responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}
	normalizeTextUsage(usage)
	if !hasValidTextUsage(usage) {
		return nil, newMissingTextUsageRetryableError(c)
	}

	responseValue := any(chatResp)
	if info.RelayFormat != types.RelayFormatOpenAI {
		targetResult, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseValue = targetResult.Value
	}
	responseBody, err := common.Marshal(responseValue)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	if info.RelayFormat == types.RelayFormatOpenAI {
		responseBody, err = ensureChatCompletionAnnotations(responseBody, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
		}
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func ensureChatCompletionAnnotations(responseBody []byte, response *dto.OpenAITextResponse) ([]byte, error) {
	if response == nil {
		return responseBody, nil
	}
	var err error
	for i := range response.Choices {
		if len(response.Choices[i].Message.Annotations) > 0 {
			continue
		}
		responseBody, err = sjson.SetRawBytes(responseBody, fmt.Sprintf("choices.%d.message.annotations", i), []byte("[]"))
		if err != nil {
			return nil, err
		}
	}
	return responseBody, nil
}

func OaiResponsesToChatBufferedStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	responseBody, streamErr := bufferResponsesStream(c, info, resp)
	if streamErr != nil {
		return nil, streamErr
	}
	return OaiResponsesToChatHandler(c, info, newBufferedJSONResponse(resp, responseBody))
}

func OaiResponsesBufferedStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	responseBody, streamErr := bufferResponsesStream(c, info, resp)
	if streamErr != nil {
		return nil, streamErr
	}
	return OaiResponsesHandler(c, info, newBufferedJSONResponse(resp, responseBody))
}

type bufferedResponsesRawEvent struct {
	Type         string          `json:"type"`
	Response     json.RawMessage `json:"response,omitempty"`
	Item         json.RawMessage `json:"item,omitempty"`
	Part         json.RawMessage `json:"part,omitempty"`
	OutputIndex  *int            `json:"output_index,omitempty"`
	ContentIndex *int            `json:"content_index,omitempty"`
}

func bufferResponsesStream(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) ([]byte, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)
	previousDisablePing := info.DisablePing
	info.DisablePing = true
	defer func() { info.DisablePing = previousDisablePing }()

	accumulator := relayconvert.NewResponsesBufferedAccumulator()
	var finalResponse []byte
	var streamErr *types.NewAPIError
	bufferedBytes := 0
	maxBufferedBytes := helper.StreamScannerMaxBufferSize()
	responseIncomplete := false
	outputItems := make(map[int]json.RawMessage)
	contentParts := make(map[int]map[int]json.RawMessage)

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		bufferedBytes += len(data)
		if bufferedBytes > maxBufferedBytes {
			err := fmt.Errorf("buffered stream response exceeds %d bytes", maxBufferedBytes)
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(err)
			return
		}
		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal buffered responses stream event: "+err.Error())
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			sr.Stop(err)
			return
		}
		var rawEvent bufferedResponsesRawEvent
		if err := common.UnmarshalJsonStr(data, &rawEvent); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			sr.Stop(err)
			return
		}
		accumulator.ProcessEvent(&streamResp)
		if rawEvent.Type == "response.output_item.done" && rawEvent.OutputIndex != nil && len(rawEvent.Item) > 0 && string(rawEvent.Item) != "null" {
			outputItems[*rawEvent.OutputIndex] = append(json.RawMessage(nil), rawEvent.Item...)
		}
		if rawEvent.Type == "response.content_part.done" && rawEvent.OutputIndex != nil && rawEvent.ContentIndex != nil && len(rawEvent.Part) > 0 && string(rawEvent.Part) != "null" {
			if contentParts[*rawEvent.OutputIndex] == nil {
				contentParts[*rawEvent.OutputIndex] = make(map[int]json.RawMessage)
			}
			contentParts[*rawEvent.OutputIndex][*rawEvent.ContentIndex] = append(json.RawMessage(nil), rawEvent.Part...)
		}
		switch streamResp.Type {
		case "response.completed", "response.done", "response.incomplete":
			if len(rawEvent.Response) > 0 && string(rawEvent.Response) != "null" {
				finalResponse = append(finalResponse[:0], rawEvent.Response...)
			}
			responseIncomplete = streamResp.Type == "response.incomplete"
			sr.Done()
		case "response.failed", "response.error":
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					sr.Stop(streamErr)
					return
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
		}
	})
	if streamErr != nil {
		return nil, streamErr
	}
	if info.StreamStatus == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("buffered stream status is unavailable"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if !info.StreamStatus.IsNormalEnd() || info.StreamStatus.HasErrors() {
		return nil, types.NewOpenAIError(fmt.Errorf("buffered stream ended unexpectedly: %s", info.StreamStatus.Summary()), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if len(finalResponse) == 0 {
		status := `"completed"`
		if responseIncomplete {
			status = `"incomplete"`
		}
		fallbackResponse := &dto.OpenAIResponsesResponse{
			ID:        helper.GetResponseID(c),
			CreatedAt: int(time.Now().Unix()),
			Model:     info.UpstreamModelName,
			Status:    []byte(status),
		}
		accumulator.SupplementResponseOutput(fallbackResponse)
		var err error
		finalResponse, err = common.Marshal(fallbackResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
		}
	} else {
		var responseFields map[string]json.RawMessage
		if err := common.Unmarshal(finalResponse, &responseFields); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		output := responseFields["output"]
		if len(output) == 0 || string(output) == "null" || string(output) == "[]" {
			fallbackResponse := &dto.OpenAIResponsesResponse{}
			accumulator.SupplementResponseOutput(fallbackResponse)
			outputJSON, err := common.Marshal(fallbackResponse.Output)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			}
			finalResponse, err = sjson.SetRawBytes(finalResponse, "output", outputJSON)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			}
		}
	}
	for outputIndex, item := range outputItems {
		var err error
		finalResponse, err = sjson.SetRawBytes(finalResponse, fmt.Sprintf("output.%d", outputIndex), item)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
		}
	}
	for outputIndex, parts := range contentParts {
		for contentIndex, part := range parts {
			var err error
			finalResponse, err = sjson.SetRawBytes(finalResponse, fmt.Sprintf("output.%d.content.%d", outputIndex, contentIndex), part)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			}
		}
	}
	return finalResponse, nil
}

func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAIResponses, info.RelayFormat, relayconvert.ResponseStreamOptions{
		ID:      responseId,
		Model:   info.UpstreamModelName,
		Created: createAt,
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	streamErr := (*types.NewAPIError)(nil)
	sawExplicitFinalUsage := false

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}

	sendGeminiResponse := func(geminiResponse *dto.GeminiChatResponse) bool {
		if geminiResponse == nil {
			return true
		}
		geminiResponseStr, err := common.Marshal(geminiResponse)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		c.Render(-1, common.CustomEvent{Data: "data: " + string(geminiResponseStr)})
		_ = helper.FlushWriter(c)
		return true
	}

	sendStreamResult := func(result relayconvert.ResponseResult) bool {
		switch value := result.Value.(type) {
		case dto.ChatCompletionsStreamResponse:
			if len(value.Choices) == 0 && value.Usage == nil {
				return true
			}
			if err := helper.ObjectData(c, &value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case *dto.ChatCompletionsStreamResponse:
			if value == nil || (len(value.Choices) == 0 && value.Usage == nil) {
				return true
			}
			if err := helper.ObjectData(c, value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case dto.ClaudeResponse:
			if err := helper.ClaudeData(c, value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case *dto.ClaudeResponse:
			if value == nil {
				return true
			}
			if err := helper.ClaudeData(c, *value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case dto.GeminiChatResponse:
			return sendGeminiResponse(&value)
		case *dto.GeminiChatResponse:
			return sendGeminiResponse(value)
		default:
			streamErr = types.NewOpenAIError(fmt.Errorf("unsupported converted stream response type %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			sr.Error(err)
			return
		}
		if streamResp.Response != nil && streamResp.Response.Usage != nil {
			sawExplicitFinalUsage = true
		}

		if streamResp.Type == "response.error" || streamResp.Type == "response.failed" {
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					sr.Stop(streamErr)
					return
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}

		results, err := relayconvert.ConvertStreamResponseChunk(c, info, state, &streamResp)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		for _, result := range results {
			if !sendStreamResult(result) {
				sr.Stop(streamErr)
				return
			}
		}
	})

	if streamErr != nil {
		return nil, streamErr
	}

	usage := state.Usage()
	if (usage == nil || usage.TotalTokens == 0) && !sawExplicitFinalUsage {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.SetUsage(usage)
	}
	normalizeTextUsage(usage)
	if !hasValidTextUsage(usage) {
		apiErr := newMissingTextUsageRetryableError(c)
		if !common.HasClientVisibleResponse(c) {
			return nil, apiErr
		}
		if err := writeChatStreamTerminalError(c, apiErr); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		return nil, apiErr
	}

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
		info.ClaudeConvertInfo.Usage = usage
	}
	finalResults, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	for _, result := range finalResults {
		if !sendStreamResult(result) {
			return nil, streamErr
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI && info.ShouldIncludeUsage && usage != nil {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseId, createAt, info.UpstreamModelName, *usage)); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		helper.Done(c)
	}
	return usage, nil
}
