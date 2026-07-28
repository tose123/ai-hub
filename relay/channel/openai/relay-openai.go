package openai

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel/openrouter"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func sendStreamData(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool) error {
	if data == "" {
		return nil
	}

	if !forceFormat && !thinkToContent {
		return helper.StringData(c, data)
	}

	var lastStreamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &lastStreamResponse); err != nil {
		return err
	}

	if !thinkToContent {
		return helper.ObjectData(c, lastStreamResponse)
	}

	hasThinkingContent := false
	hasContent := false
	var thinkingContent strings.Builder
	for _, choice := range lastStreamResponse.Choices {
		if len(choice.Delta.GetReasoningContent()) > 0 {
			hasThinkingContent = true
			thinkingContent.WriteString(choice.Delta.GetReasoningContent())
		}
		if len(choice.Delta.GetContentString()) > 0 {
			hasContent = true
		}
	}

	// Handle think to content conversion
	if info.ThinkingContentInfo.IsFirstThinkingContent {
		if hasThinkingContent {
			response := lastStreamResponse.Copy()
			for i := range response.Choices {
				// send `think` tag with thinking content
				response.Choices[i].Delta.SetContentString("<think>\n" + thinkingContent.String())
				response.Choices[i].Delta.ReasoningContent = nil
				response.Choices[i].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.IsFirstThinkingContent = false
			info.ThinkingContentInfo.HasSentThinkingContent = true
			return helper.ObjectData(c, response)
		}
	}

	if lastStreamResponse.Choices == nil || len(lastStreamResponse.Choices) == 0 {
		return helper.ObjectData(c, lastStreamResponse)
	}

	// Process each choice
	for i, choice := range lastStreamResponse.Choices {
		// Handle transition from thinking to content
		// only send `</think>` tag when previous thinking content has been sent
		if hasContent && !info.ThinkingContentInfo.SendLastThinkingContent && info.ThinkingContentInfo.HasSentThinkingContent {
			response := lastStreamResponse.Copy()
			for j := range response.Choices {
				response.Choices[j].Delta.SetContentString("\n</think>\n")
				response.Choices[j].Delta.ReasoningContent = nil
				response.Choices[j].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.SendLastThinkingContent = true
			helper.ObjectData(c, response)
		}

		// Convert reasoning content to regular content if any
		if len(choice.Delta.GetReasoningContent()) > 0 {
			lastStreamResponse.Choices[i].Delta.SetContentString(choice.Delta.GetReasoningContent())
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		} else if !hasThinkingContent && !hasContent {
			// flush thinking content
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		}
	}

	return helper.ObjectData(c, lastStreamResponse)
}

func newBufferedJSONResponse(resp *http.Response, body []byte) *http.Response {
	bufferedResp := new(http.Response)
	*bufferedResp = *resp
	bufferedResp.Header = resp.Header.Clone()
	bufferedResp.Header.Set("Content-Type", "application/json")
	bufferedResp.Header.Del("Content-Length")
	bufferedResp.ContentLength = int64(len(body))
	bufferedResp.TransferEncoding = nil
	bufferedResp.Body = io.NopCloser(bytes.NewReader(body))
	return bufferedResp
}

func OaiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	model := info.UpstreamModelName
	var responseId string
	var createAt int64 = 0
	var systemFingerprint string
	var containStreamUsage bool
	var sawExplicitStreamUsage bool
	var responseTextBuilder strings.Builder
	var toolCount int
	var usage = &dto.Usage{}
	var lastStreamData string
	var secondLastStreamData string // 存储倒数第二个stream data，用于音频模型
	seenStreamToolCalls := make(map[string]struct{})
	var streamFunctionCallNames []string

	// 检查是否为音频模型
	isAudioModel := strings.Contains(strings.ToLower(model), "audio")

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if lastStreamData != "" {
			if err := HandleStreamFormat(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent); err != nil {
				common.SysLog("error handling stream format: " + err.Error())
				sr.Error(err)
			}
		}
		if len(data) > 0 {
			// 对音频模型，保存倒数第二个stream data
			if isAudioModel && lastStreamData != "" {
				secondLastStreamData = lastStreamData
			}

			lastStreamData = data
			collectStreamFunctionCallNames(data, seenStreamToolCalls, &streamFunctionCallNames)
			if err := processTokenData(info.RelayMode, data, &responseTextBuilder, &toolCount); err != nil {
				logger.LogError(c, "error processing stream token data: "+err.Error())
				sr.Error(err)
			}
		}
	})

	// 对音频模型，从倒数第二个stream data中提取usage信息
	if isAudioModel && secondLastStreamData != "" {
		var streamResp struct {
			Usage *dto.Usage `json:"usage"`
		}
		err := common.Unmarshal([]byte(secondLastStreamData), &streamResp)
		if err == nil && streamResp.Usage != nil && service.ValidUsage(streamResp.Usage) {
			usage = streamResp.Usage
			containStreamUsage = true

			if common.DebugEnabled {
				logger.LogDebug(c, "Audio model usage extracted from second last SSE: PromptTokens=%d, CompletionTokens=%d, TotalTokens=%d, InputTokens=%d, OutputTokens=%d",
					usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
					usage.InputTokens, usage.OutputTokens)
			}
		}
	}

	// 处理最后的响应
	shouldSendLastResp := true
	if err := handleLastResponse(lastStreamData, &responseId, &createAt, &systemFingerprint, &model, &usage,
		&containStreamUsage, info, &shouldSendLastResp); err != nil {
		logger.LogError(c, fmt.Sprintf("error handling last response: %s, lastStreamData: [%s]", err.Error(), lastStreamData))
	}
	var lastStreamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(lastStreamData), &lastStreamResponse); err == nil && lastStreamResponse.Usage != nil {
		sawExplicitStreamUsage = true
	}

	if !containStreamUsage && !sawExplicitStreamUsage {
		usage = service.ResponseText2Usage(c, responseTextBuilder.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		usage.CompletionTokens += toolCount * 7
	}

	applyUsagePostProcessing(info, usage, common.StringToByteSlice(lastStreamData))
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

	if info.RelayFormat == types.RelayFormatOpenAI && shouldSendLastResp {
		_ = sendStreamData(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
	}

	for _, name := range streamFunctionCallNames {
		info.CountBillableToolCall(dto.BuildInCallFunctionCall, name)
	}

	HandleFinalResponse(c, info, lastStreamData, responseId, createAt, model, systemFingerprint, usage, containStreamUsage)

	return usage, nil
}

type bufferedChatToolCall struct {
	id        string
	typ       any
	name      strings.Builder
	arguments strings.Builder
}

type bufferedChatChoice struct {
	role                  string
	content               strings.Builder
	reasoning             strings.Builder
	refusal               strings.Builder
	functionCallName      strings.Builder
	functionCallArguments strings.Builder
	finishReason          string
	toolCalls             map[int]*bufferedChatToolCall
	logprobsSeen          bool
	logprobsContentSeen   bool
	logprobsRefusalSeen   bool
	logprobsContent       []any
	logprobsRefusal       []any
}

func appendChatLogprobs(choice *bufferedChatChoice, logprobs *any) {
	if choice == nil || logprobs == nil || *logprobs == nil {
		return
	}
	value, ok := (*logprobs).(map[string]interface{})
	if !ok {
		return
	}
	choice.logprobsSeen = true
	if content, ok := value["content"].([]interface{}); ok {
		choice.logprobsContentSeen = true
		if choice.logprobsContent == nil {
			choice.logprobsContent = make([]any, 0, len(content))
		}
		choice.logprobsContent = append(choice.logprobsContent, content...)
	}
	if refusal, ok := value["refusal"].([]interface{}); ok {
		choice.logprobsRefusalSeen = true
		if choice.logprobsRefusal == nil {
			choice.logprobsRefusal = make([]any, 0, len(refusal))
		}
		choice.logprobsRefusal = append(choice.logprobsRefusal, refusal...)
	}
}

func OaiBufferedStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)
	previousDisablePing := info.DisablePing
	info.DisablePing = true
	defer func() { info.DisablePing = previousDisablePing }()

	choices := make(map[int]*bufferedChatChoice)
	response := dto.OpenAITextResponse{Object: "chat.completion"}
	var responseTextBuilder strings.Builder
	var usage *dto.Usage
	var streamErr *types.NewAPIError
	var toolCount int
	bufferedBytes := 0
	maxBufferedBytes := helper.StreamScannerMaxBufferSize()

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		bufferedBytes += len(data)
		if bufferedBytes > maxBufferedBytes {
			err := fmt.Errorf("buffered stream response exceeds %d bytes", maxBufferedBytes)
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(err)
			return
		}

		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			sr.Stop(err)
			return
		}
		if err := ProcessStreamResponse(chunk, &responseTextBuilder, &toolCount); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			sr.Stop(err)
			return
		}
		if chunk.Id != "" {
			response.Id = chunk.Id
		}
		if chunk.Created != 0 {
			response.Created = chunk.Created
		}
		if chunk.Model != "" {
			response.Model = chunk.Model
		}
		if chunk.SystemFingerprint != nil {
			response.SystemFingerprint = chunk.SystemFingerprint
		}
		if len(chunk.ServiceTier) > 0 {
			response.ServiceTier = chunk.ServiceTier
		}
		if service.ValidUsage(chunk.Usage) {
			usage = chunk.Usage
		}

		for _, chunkChoice := range chunk.Choices {
			choice := choices[chunkChoice.Index]
			if choice == nil {
				choice = &bufferedChatChoice{toolCalls: make(map[int]*bufferedChatToolCall)}
				choices[chunkChoice.Index] = choice
			}
			if chunkChoice.Delta.Role != "" {
				choice.role = chunkChoice.Delta.Role
			}
			choice.content.WriteString(chunkChoice.Delta.GetContentString())
			choice.reasoning.WriteString(chunkChoice.Delta.GetReasoningContent())
			if chunkChoice.Delta.Refusal != nil {
				choice.refusal.WriteString(*chunkChoice.Delta.Refusal)
			}
			if chunkChoice.Delta.FunctionCall != nil {
				choice.functionCallName.WriteString(chunkChoice.Delta.FunctionCall.Name)
				choice.functionCallArguments.WriteString(chunkChoice.Delta.FunctionCall.Arguments)
			}
			appendChatLogprobs(choice, chunkChoice.Logprobs)
			if chunkChoice.FinishReason != nil {
				choice.finishReason = *chunkChoice.FinishReason
			}
			for _, chunkTool := range chunkChoice.Delta.ToolCalls {
				toolIndex := 0
				if chunkTool.Index != nil {
					toolIndex = *chunkTool.Index
				}
				tool := choice.toolCalls[toolIndex]
				if tool == nil {
					tool = &bufferedChatToolCall{}
					choice.toolCalls[toolIndex] = tool
				}
				if chunkTool.ID != "" {
					tool.id = chunkTool.ID
				}
				if chunkTool.Type != nil {
					tool.typ = chunkTool.Type
				}
				tool.name.WriteString(chunkTool.Function.Name)
				tool.arguments.WriteString(chunkTool.Function.Arguments)
			}
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
	if len(choices) == 0 {
		return nil, types.NewOpenAIError(fmt.Errorf("buffered stream returned no choices"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	choiceIndexes := make([]int, 0, len(choices))
	for index := range choices {
		choiceIndexes = append(choiceIndexes, index)
	}
	sort.Ints(choiceIndexes)
	response.Choices = make([]dto.OpenAITextResponseChoice, 0, len(choiceIndexes))
	for _, index := range choiceIndexes {
		bufferedChoice := choices[index]
		message := dto.Message{Role: bufferedChoice.role}
		if message.Role == "" {
			message.Role = "assistant"
		}
		if bufferedChoice.content.Len() == 0 && (len(bufferedChoice.toolCalls) > 0 || bufferedChoice.refusal.Len() > 0) {
			message.SetNullContent()
		} else {
			message.SetStringContent(bufferedChoice.content.String())
		}
		if bufferedChoice.refusal.Len() > 0 {
			refusal := bufferedChoice.refusal.String()
			message.Refusal = &refusal
		}
		if bufferedChoice.reasoning.Len() > 0 {
			reasoning := bufferedChoice.reasoning.String()
			message.ReasoningContent = &reasoning
		}
		if bufferedChoice.functionCallName.Len() > 0 || bufferedChoice.functionCallArguments.Len() > 0 {
			functionCall, err := common.Marshal(dto.FunctionResponse{
				Name:      bufferedChoice.functionCallName.String(),
				Arguments: bufferedChoice.functionCallArguments.String(),
			})
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			}
			message.FunctionCall = functionCall
		}
		if len(bufferedChoice.toolCalls) > 0 {
			toolIndexes := make([]int, 0, len(bufferedChoice.toolCalls))
			for toolIndex := range bufferedChoice.toolCalls {
				toolIndexes = append(toolIndexes, toolIndex)
			}
			sort.Ints(toolIndexes)
			toolCalls := make([]dto.ToolCallResponse, 0, len(toolIndexes))
			for _, toolIndex := range toolIndexes {
				bufferedTool := bufferedChoice.toolCalls[toolIndex]
				toolCalls = append(toolCalls, dto.ToolCallResponse{
					ID:   bufferedTool.id,
					Type: bufferedTool.typ,
					Function: dto.FunctionResponse{
						Name:      bufferedTool.name.String(),
						Arguments: bufferedTool.arguments.String(),
					},
				})
			}
			toolCallsJSON, err := common.Marshal(toolCalls)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			}
			message.ToolCalls = toolCallsJSON
		}
		var logprobs *any
		if bufferedChoice.logprobsSeen {
			logprobsValue := map[string]any{}
			if bufferedChoice.logprobsContentSeen {
				logprobsValue["content"] = bufferedChoice.logprobsContent
			}
			if bufferedChoice.logprobsRefusalSeen {
				logprobsValue["refusal"] = bufferedChoice.logprobsRefusal
			}
			logprobsAny := any(logprobsValue)
			logprobs = &logprobsAny
		}
		response.Choices = append(response.Choices, dto.OpenAITextResponseChoice{
			Index:        index,
			Message:      message,
			Logprobs:     logprobs,
			FinishReason: bufferedChoice.finishReason,
		})
	}

	if response.Id == "" {
		response.Id = helper.GetResponseID(c)
	}
	if response.Created == nil {
		response.Created = info.StartTime.Unix()
	}
	if response.Model == "" {
		response.Model = info.UpstreamModelName
	}
	if !service.ValidUsage(usage) {
		usage = service.ResponseText2Usage(c, responseTextBuilder.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		usage.CompletionTokens += toolCount * 7
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	response.Usage = *usage
	responseBody, err := common.Marshal(response)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	return OpenaiHandler(c, info, newBufferedJSONResponse(resp, responseBody))
}

func OpenaiHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	var simpleResponse dto.OpenAITextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	logger.LogDebug(c, "upstream response body: %s", responseBody)
	// Unmarshal to simpleResponse
	if info.ChannelType == constant.ChannelTypeOpenRouter && info.ChannelOtherSettings.IsOpenRouterEnterprise() {
		// 尝试解析为 openrouter enterprise
		var enterpriseResponse openrouter.OpenRouterEnterpriseResponse
		err = common.Unmarshal(responseBody, &enterpriseResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if enterpriseResponse.Success {
			responseBody = enterpriseResponse.Data
		} else {
			logger.LogError(c, fmt.Sprintf("openrouter enterprise response success=false, data: %s", enterpriseResponse.Data))
			return nil, types.NewOpenAIError(fmt.Errorf("openrouter response success=false"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	err = common.Unmarshal(responseBody, &simpleResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := simpleResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	for _, choice := range simpleResponse.Choices {
		if choice.FinishReason == constant.FinishReasonContentFilter {
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "openai_finish_reason=content_filter")
			break
		}
	}

	for _, choice := range simpleResponse.Choices {
		for _, tc := range choice.Message.ParseToolCalls() {
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, tc.Function.Name)
		}
	}

	forceFormat := false
	if info.ChannelSetting.ForceFormat {
		forceFormat = true
	}

	usageModified := false
	if simpleResponse.Usage.PromptTokens == 0 {
		completionTokens := simpleResponse.Usage.CompletionTokens
		if completionTokens == 0 {
			for _, choice := range simpleResponse.Choices {
				ctkm := service.CountTextToken(choice.Message.StringContent()+choice.Message.GetReasoningContent(), info.UpstreamModelName)
				completionTokens += ctkm
			}
		}
		simpleResponse.Usage = dto.Usage{
			PromptTokens:     info.GetEstimatePromptTokens(),
			CompletionTokens: completionTokens,
			TotalTokens:      info.GetEstimatePromptTokens() + completionTokens,
		}
		usageModified = true
	}

	applyUsagePostProcessing(info, &simpleResponse.Usage, responseBody)
	normalizeTextUsage(&simpleResponse.Usage)
	if !hasValidTextUsage(&simpleResponse.Usage) {
		return nil, newMissingTextUsageRetryableError(c)
	}

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if usageModified {
			var bodyMap map[string]interface{}
			err = common.Unmarshal(responseBody, &bodyMap)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			bodyMap["usage"] = simpleResponse.Usage
			responseBody, _ = common.Marshal(bodyMap)
		}
		if forceFormat {
			responseBody, err = common.Marshal(simpleResponse)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		} else {
			break
		}
	case types.RelayFormatClaude:
		convertResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatClaude, &simpleResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		claudeRespStr, err := common.Marshal(convertResult.Value)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = claudeRespStr
	case types.RelayFormatGemini:
		convertResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatGemini, &simpleResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		geminiRespStr, err := common.Marshal(convertResult.Value)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = geminiRespStr
	}
	if info.RelayFormat == types.RelayFormatOpenAI && info.UpstreamIsStream && !info.IsStream {
		responseBody, err = ensureChatCompletionAnnotations(responseBody, &simpleResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
		}
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &simpleResponse.Usage, nil
}
