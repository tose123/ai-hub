package service

import (
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

type jsonRawMessageAlias = []byte

type sensitiveInfoRedactionRule struct {
	name        string
	placeholder string
	pattern     *regexp.Regexp
}

var sensitiveInfoRedactionRules = []sensitiveInfoRedactionRule{
	{
		name:        "api_key",
		placeholder: "[API_KEY_REDACTED]",
		pattern:     regexp.MustCompile(`(?i)(?:\b(?:x[-_ ]?api[-_ ]?key|api[-_ ]?key|api key)\b\s*[:=]\s*["']?(?:sk-[A-Za-z0-9_-]{16,}|[A-Za-z0-9+/=_-]{16,})["']?|\bsk-[A-Za-z0-9_-]{16,}\b)`),
	},
	{
		name:        "bearer_token",
		placeholder: "[BEARER_TOKEN_REDACTED]",
		pattern:     regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-+/=]{20,}\b`),
	},
	{
		name:        "wallet_private_key",
		placeholder: "[WALLET_PRIVATE_KEY_REDACTED]",
		pattern:     regexp.MustCompile(`(?i)(?:\b(?:wallet[_ -]?private[_ -]?key|private[_ -]?key|wallet private key)\b\s*[:=]\s*["']?(?:0x)?[A-Fa-f0-9]{64}["']?|\b(?:0x)?[A-Fa-f0-9]{64}\b)`),
	},
	{
		name:        "mnemonic_phrase",
		placeholder: "[MNEMONIC_PHRASE_REDACTED]",
		pattern:     regexp.MustCompile(`(?i)\b(?:mnemonic|seed phrase|助记词)\b\s*[:=]\s*(?:["']?)(?:[A-Za-z]+\s+){11,23}[A-Za-z]+(?:["']?)`),
	},
}

func RedactSensitiveInfoText(text string) (redacted string, changed bool) {
	if len(text) == 0 {
		return text, false
	}

	redacted = text
	for _, rule := range sensitiveInfoRedactionRules {
		updated := rule.pattern.ReplaceAllString(redacted, rule.placeholder)
		if updated != redacted {
			redacted = updated
			changed = true
		}
	}
	return redacted, changed
}

func RedactSensitiveInfoAny(v any) (redacted any, changed bool) {
	switch value := v.(type) {
	case string:
		return RedactSensitiveInfoText(value)
	case []string:
		redactedSlice := make([]string, len(value))
		for i, item := range value {
			redactedItem, itemChanged := RedactSensitiveInfoAny(item)
			if str, ok := redactedItem.(string); ok {
				redactedSlice[i] = str
			} else {
				redactedSlice[i] = item
			}
			changed = changed || itemChanged
		}
		return redactedSlice, changed
	case []any:
		redactedSlice := make([]any, len(value))
		for i, item := range value {
			redactedItem, itemChanged := RedactSensitiveInfoAny(item)
			redactedSlice[i] = redactedItem
			changed = changed || itemChanged
		}
		return redactedSlice, changed
	case map[string]any:
		redactedMap := make(map[string]any, len(value))
		for key, item := range value {
			redactedItem, itemChanged := RedactSensitiveInfoAny(item)
			redactedMap[key] = redactedItem
			changed = changed || itemChanged
		}
		return redactedMap, changed
	default:
		return v, false
	}
}

func RedactSensitiveInfoRequest(request dto.Request) bool {
	switch req := request.(type) {
	case *dto.GeneralOpenAIRequest:
		return RedactSensitiveInfoOpenAIRequest(req)
	case *dto.OpenAIResponsesRequest:
		return RedactSensitiveInfoOpenAIResponsesRequest(req)
	case *dto.OpenAIResponsesCompactionRequest:
		return RedactSensitiveInfoOpenAIResponsesCompactionRequest(req)
	case *dto.ImageRequest:
		return RedactSensitiveInfoImageRequest(req)
	case *dto.AudioRequest:
		return RedactSensitiveInfoAudioRequest(req)
	case *dto.EmbeddingRequest:
		return RedactSensitiveInfoEmbeddingRequest(req)
	case *dto.RerankRequest:
		return RedactSensitiveInfoRerankRequest(req)
	case *dto.ClaudeRequest:
		return RedactSensitiveInfoClaudeRequest(req)
	case *dto.GeminiChatRequest:
		return RedactSensitiveInfoGeminiChatRequest(req)
	case *dto.GeminiEmbeddingRequest:
		return RedactSensitiveInfoGeminiEmbeddingRequest(req)
	case *dto.GeminiBatchEmbeddingRequest:
		return RedactSensitiveInfoGeminiBatchEmbeddingRequest(req)
	default:
		return false
	}
}

func RedactSensitiveInfoOpenAIRequest(request *dto.GeneralOpenAIRequest) bool {
	if request == nil {
		return false
	}

	changed := false
	redactField := func(value any) any {
		redacted, fieldChanged := redactOpenAIRequestAny(value)
		if fieldChanged {
			changed = true
		}
		return redacted
	}
	redactRaw := func(raw []byte) []byte {
		redacted, fieldChanged := redactOpenAIRequestRawJSON(raw)
		if fieldChanged {
			changed = true
		}
		return redacted
	}
	redactText := func(value string) string {
		redacted, fieldChanged := RedactSensitiveInfoText(value)
		if fieldChanged {
			changed = true
		}
		return redacted
	}

	if request.Prompt != nil {
		request.Prompt = redactField(request.Prompt)
	}
	if request.Prefix != nil {
		request.Prefix = redactField(request.Prefix)
	}
	if request.Suffix != nil {
		request.Suffix = redactField(request.Suffix)
	}
	if request.Input != nil {
		request.Input = redactField(request.Input)
	}
	if request.Stop != nil {
		request.Stop = redactField(request.Stop)
	}
	if request.ToolChoice != nil {
		request.ToolChoice = redactField(request.ToolChoice)
	}
	request.Instruction = redactText(request.Instruction)
	request.ReasoningEffort = redactText(request.ReasoningEffort)
	request.Size = redactText(request.Size)
	request.PromptCacheKey = redactText(request.PromptCacheKey)

	request.Verbosity = redactRaw(request.Verbosity)
	request.Functions = redactRaw(request.Functions)
	request.EncodingFormat = redactRaw(request.EncodingFormat)
	request.FunctionCall = redactRaw(request.FunctionCall)
	request.User = redactRaw(request.User)
	request.ServiceTier = redactRaw(request.ServiceTier)
	request.Modalities = redactRaw(request.Modalities)
	request.Audio = redactRaw(request.Audio)
	request.SafetyIdentifier = redactRaw(request.SafetyIdentifier)
	request.Store = redactRaw(request.Store)
	request.PromptCacheRetention = redactRaw(request.PromptCacheRetention)
	request.LogitBias = redactRaw(request.LogitBias)
	request.Metadata = redactRaw(request.Metadata)
	request.Prediction = redactRaw(request.Prediction)
	request.ExtraBody = redactRaw(request.ExtraBody)
	request.SearchParameters = redactRaw(request.SearchParameters)
	request.Usage = redactRaw(request.Usage)
	request.Reasoning = redactRaw(request.Reasoning)
	request.VlHighResolutionImages = redactRaw(request.VlHighResolutionImages)
	request.EnableThinking = redactRaw(request.EnableThinking)
	request.ChatTemplateKwargs = redactRaw(request.ChatTemplateKwargs)
	request.EnableSearch = redactRaw(request.EnableSearch)
	request.Think = redactRaw(request.Think)
	request.WebSearch = redactRaw(request.WebSearch)
	request.THINKING = redactRaw(request.THINKING)
	request.SearchDomainFilter = redactRaw(request.SearchDomainFilter)
	request.SearchRecencyFilter = redactRaw(request.SearchRecencyFilter)
	request.SearchMode = redactRaw(request.SearchMode)
	request.ReasoningSplit = redactRaw(request.ReasoningSplit)

	if request.ResponseFormat != nil {
		request.ResponseFormat.JsonSchema = redactRaw(request.ResponseFormat.JsonSchema)
	}
	if request.WebSearchOptions != nil {
		request.WebSearchOptions.SearchContextSize = redactText(request.WebSearchOptions.SearchContextSize)
		request.WebSearchOptions.UserLocation = redactRaw(request.WebSearchOptions.UserLocation)
	}

	for i := range request.Messages {
		if redactOpenAIRequestMessage(&request.Messages[i]) {
			changed = true
		}
	}

	for i := range request.Tools {
		request.Tools[i].Function.Description = redactText(request.Tools[i].Function.Description)
		request.Tools[i].Function.Arguments = redactText(request.Tools[i].Function.Arguments)
		if redacted, fieldChanged := redactOpenAIRequestToolParameters(request.Tools[i].Function.Parameters); fieldChanged {
			request.Tools[i].Function.Parameters = redacted
			changed = true
		}
		request.Tools[i].Custom = redactRaw(request.Tools[i].Custom)
	}

	return changed
}

func RedactSensitiveInfoOpenAIResponsesRequest(request *dto.OpenAIResponsesRequest) bool {
	if request == nil {
		return false
	}

	changed := false
	redactRaw := func(raw []byte, preserve func(map[string]any, string) bool) []byte {
		if preserve == nil {
			preserve = shouldPreserveOpenAIOpaqueField
		}
		redacted, fieldChanged := redactSensitiveInfoRawJSON(raw, preserve)
		if fieldChanged {
			changed = true
		}
		return redacted
	}
	redactRawWithPath := func(raw []byte, preserve func([]map[string]any, map[string]any, string, any) bool) []byte {
		redacted, fieldChanged := redactSensitiveInfoRawJSONWithPreservePath(raw, preserve)
		if fieldChanged {
			changed = true
		}
		return redacted
	}

	request.Input = redactRawWithPath(request.Input, shouldPreserveOpenAIResponsesInputFieldPath)
	request.Instructions = redactRaw(request.Instructions, nil)
	request.Metadata = redactRaw(request.Metadata, nil)
	request.Text = redactRaw(request.Text, nil)
	request.ToolChoice = redactRaw(request.ToolChoice, nil)
	request.Tools = redactRaw(request.Tools, nil)
	request.Prompt = redactRaw(request.Prompt, nil)
	request.User = redactRaw(request.User, nil)
	request.Conversation = redactRaw(request.Conversation, nil)
	request.ContextManagement = redactRaw(request.ContextManagement, nil)
	request.ParallelToolCalls = redactRaw(request.ParallelToolCalls, nil)
	request.Truncation = redactRaw(request.Truncation, nil)
	request.EnableThinking = redactRaw(request.EnableThinking, nil)
	request.Preset = redactRaw(request.Preset, nil)
	request.Store = redactRaw(request.Store, nil)
	request.PromptCacheKey = redactRaw(request.PromptCacheKey, nil)
	request.PromptCacheRetention = redactRaw(request.PromptCacheRetention, nil)
	request.SafetyIdentifier = redactRaw(request.SafetyIdentifier, nil)
	if request.Reasoning != nil {
		if redacted, fieldChanged := RedactSensitiveInfoText(request.Reasoning.Summary); fieldChanged {
			request.Reasoning.Summary = redacted
			changed = true
		}
	}
	return changed
}

func RedactSensitiveInfoOpenAIResponsesCompactionRequest(request *dto.OpenAIResponsesCompactionRequest) bool {
	if request == nil {
		return false
	}

	changed := false
	if redacted, fieldChanged := redactSensitiveInfoRawJSONWithPreservePath(request.Input, shouldPreserveOpenAIResponsesInputFieldPath); fieldChanged {
		request.Input = redacted
		changed = true
	}
	if redacted, fieldChanged := redactSensitiveInfoRawJSON(request.Instructions, shouldPreserveOpenAIOpaqueField); fieldChanged {
		request.Instructions = redacted
		changed = true
	}
	return changed
}

func RedactSensitiveInfoEmbeddingRequest(request *dto.EmbeddingRequest) bool {
	if request == nil {
		return false
	}

	changed := false
	if request.Input != nil {
		if redacted, fieldChanged := RedactSensitiveInfoAny(request.Input); fieldChanged {
			request.Input = redacted
			changed = true
		}
	}
	if redacted, fieldChanged := RedactSensitiveInfoText(request.User); fieldChanged {
		request.User = redacted
		changed = true
	}
	return changed
}

func RedactSensitiveInfoRerankRequest(request *dto.RerankRequest) bool {
	if request == nil {
		return false
	}

	changed := false
	if redacted, fieldChanged := RedactSensitiveInfoAny(request.Documents); fieldChanged {
		if documents, ok := redacted.([]any); ok {
			request.Documents = documents
			changed = true
		}
	}
	if redacted, fieldChanged := RedactSensitiveInfoText(request.Query); fieldChanged {
		request.Query = redacted
		changed = true
	}
	return changed
}

func RedactSensitiveInfoImageRequest(request *dto.ImageRequest) bool {
	if request == nil {
		return false
	}

	changed := false
	if redacted, fieldChanged := RedactSensitiveInfoText(request.Prompt); fieldChanged {
		request.Prompt = redacted
		changed = true
	}
	for _, field := range []*string{&request.Size, &request.Quality, &request.ResponseFormat} {
		if redacted, fieldChanged := RedactSensitiveInfoText(*field); fieldChanged {
			*field = redacted
			changed = true
		}
	}
	redactRawField := func(field *jsonRawMessageAlias) {
		if redacted, fieldChanged := redactSensitiveInfoRawJSON(*field, nil); fieldChanged {
			*field = redacted
			changed = true
		}
	}
	redactRawField((*jsonRawMessageAlias)(&request.Style))
	redactRawField((*jsonRawMessageAlias)(&request.User))
	redactRawField((*jsonRawMessageAlias)(&request.ExtraFields))
	redactRawField((*jsonRawMessageAlias)(&request.Background))
	redactRawField((*jsonRawMessageAlias)(&request.Moderation))
	redactRawField((*jsonRawMessageAlias)(&request.OutputFormat))
	redactRawField((*jsonRawMessageAlias)(&request.OutputCompression))
	redactRawField((*jsonRawMessageAlias)(&request.PartialImages))
	redactRawField((*jsonRawMessageAlias)(&request.InputFidelity))
	redactRawField((*jsonRawMessageAlias)(&request.WatermarkEnabled))
	redactRawField((*jsonRawMessageAlias)(&request.UserId))
	for key, raw := range request.Extra {
		if shouldPreserveImageRequestExtraField(key) {
			continue
		}
		if redacted, fieldChanged := redactSensitiveInfoRawJSON(raw, nil); fieldChanged {
			request.Extra[key] = redacted
			changed = true
		}
	}
	return changed
}

func RedactSensitiveInfoAudioRequest(request *dto.AudioRequest) bool {
	if request == nil {
		return false
	}

	changed := false
	for _, field := range []*string{&request.Input, &request.Instructions, &request.Voice, &request.ResponseFormat, &request.StreamFormat} {
		if redacted, fieldChanged := RedactSensitiveInfoText(*field); fieldChanged {
			*field = redacted
			changed = true
		}
	}
	redactAudioRaw := func(field *jsonRawMessageAlias) {
		if redacted, fieldChanged := redactSensitiveInfoRawJSON(*field, nil); fieldChanged {
			*field = redacted
			changed = true
		}
	}
	redactAudioRaw((*jsonRawMessageAlias)(&request.Metadata))
	redactAudioRaw((*jsonRawMessageAlias)(&request.TaskType))
	redactAudioRaw((*jsonRawMessageAlias)(&request.Language))
	redactAudioRaw((*jsonRawMessageAlias)(&request.RefText))
	redactAudioRaw((*jsonRawMessageAlias)(&request.XVectorOnlyMode))
	redactAudioRaw((*jsonRawMessageAlias)(&request.MaxNewTokens))
	redactAudioRaw((*jsonRawMessageAlias)(&request.InitialCodecChunkFrames))
	return changed
}

func RedactSensitiveInfoClaudeRequest(request *dto.ClaudeRequest) bool {
	if request == nil {
		return false
	}

	changed := false
	if redacted, fieldChanged := RedactSensitiveInfoText(request.Prompt); fieldChanged {
		request.Prompt = redacted
		changed = true
	}
	for i, stop := range request.StopSequences {
		if redacted, fieldChanged := RedactSensitiveInfoText(stop); fieldChanged {
			request.StopSequences[i] = redacted
			changed = true
		}
	}
	if redacted, fieldChanged := redactClaudeContent(request.System); fieldChanged {
		request.System = redacted
		changed = true
	}
	for i := range request.Messages {
		if redacted, fieldChanged := redactClaudeContent(request.Messages[i].Content); fieldChanged {
			request.Messages[i].Content = redacted
			changed = true
		}
	}
	if redacted, fieldChanged := RedactSensitiveInfoAny(request.Tools); fieldChanged {
		request.Tools = redacted
		changed = true
	}
	if redacted, fieldChanged := RedactSensitiveInfoAny(request.ToolChoice); fieldChanged {
		request.ToolChoice = redacted
		changed = true
	}
	for _, field := range []*jsonRawMessageAlias{
		(*jsonRawMessageAlias)(&request.CacheControl),
		(*jsonRawMessageAlias)(&request.ContextManagement),
		(*jsonRawMessageAlias)(&request.OutputConfig),
		(*jsonRawMessageAlias)(&request.OutputFormat),
		(*jsonRawMessageAlias)(&request.Container),
		(*jsonRawMessageAlias)(&request.McpServers),
		(*jsonRawMessageAlias)(&request.Metadata),
		(*jsonRawMessageAlias)(&request.Speed),
	} {
		if redacted, fieldChanged := redactSensitiveInfoRawJSON(*field, nil); fieldChanged {
			*field = redacted
			changed = true
		}
	}
	return changed
}

func RedactSensitiveInfoGeminiChatRequest(request *dto.GeminiChatRequest) bool {
	if request == nil {
		return false
	}

	changed := false
	for i := range request.Requests {
		if RedactSensitiveInfoGeminiChatRequest(&request.Requests[i]) {
			changed = true
		}
	}
	for i := range request.Contents {
		if redactGeminiChatContent(&request.Contents[i]) {
			changed = true
		}
	}
	if redactGeminiGenerationConfig(&request.GenerationConfig) {
		changed = true
	}
	if request.SystemInstructions != nil && redactGeminiChatContent(request.SystemInstructions) {
		changed = true
	}
	if redacted, fieldChanged := redactGeminiRawJSON(request.Tools); fieldChanged {
		request.Tools = redacted
		changed = true
	}
	if request.ToolConfig != nil && request.ToolConfig.RetrievalConfig != nil {
		if redacted, fieldChanged := RedactSensitiveInfoText(request.ToolConfig.RetrievalConfig.LanguageCode); fieldChanged {
			request.ToolConfig.RetrievalConfig.LanguageCode = redacted
			changed = true
		}
	}
	if redacted, fieldChanged := RedactSensitiveInfoText(request.CachedContent); fieldChanged {
		request.CachedContent = redacted
		changed = true
	}
	return changed
}

func RedactSensitiveInfoGeminiEmbeddingRequest(request *dto.GeminiEmbeddingRequest) bool {
	if request == nil {
		return false
	}

	changed := redactGeminiChatContent(&request.Content)
	if redacted, fieldChanged := RedactSensitiveInfoText(request.TaskType); fieldChanged {
		request.TaskType = redacted
		changed = true
	}
	if redacted, fieldChanged := RedactSensitiveInfoText(request.Title); fieldChanged {
		request.Title = redacted
		changed = true
	}
	return changed
}

func RedactSensitiveInfoGeminiBatchEmbeddingRequest(request *dto.GeminiBatchEmbeddingRequest) bool {
	if request == nil {
		return false
	}

	changed := false
	for _, embeddingRequest := range request.Requests {
		if RedactSensitiveInfoGeminiEmbeddingRequest(embeddingRequest) {
			changed = true
		}
	}
	return changed
}

func redactOpenAIRequestMessage(message *dto.Message) bool {
	if message == nil {
		return false
	}

	changed := false
	if message.Content != nil {
		if message.IsStringContent() {
			if redacted, fieldChanged := RedactSensitiveInfoText(message.StringContent()); fieldChanged {
				message.SetStringContent(redacted)
				changed = true
			}
		} else {
			switch content := message.Content.(type) {
			case []dto.MediaContent:
				redacted := make([]dto.MediaContent, len(content))
				copy(redacted, content)
				for i := range redacted {
					if redactOpenAIRequestMediaContent(&redacted[i]) {
						changed = true
					}
				}
				if changed {
					message.SetMediaContent(redacted)
				}
			case []any:
				if redacted, fieldChanged := redactOpenAIRequestContentAny(content); fieldChanged {
					message.Content = redacted
					changed = true
				}
			case map[string]any:
				if redacted, fieldChanged := redactOpenAIRequestContentAny(content); fieldChanged {
					message.Content = redacted
					changed = true
				}
			default:
				_ = message.ParseContent()
			}
		}
	}

	if message.ReasoningContent != nil {
		if redacted, fieldChanged := RedactSensitiveInfoText(*message.ReasoningContent); fieldChanged {
			*message.ReasoningContent = redacted
			changed = true
		}
	}
	if message.Reasoning != nil {
		if redacted, fieldChanged := RedactSensitiveInfoText(*message.Reasoning); fieldChanged {
			*message.Reasoning = redacted
			changed = true
		}
	}
	if redacted, fieldChanged := redactOpenAIRequestRawJSON(message.ToolCalls); fieldChanged {
		message.ToolCalls = redacted
		changed = true
	}

	return changed
}

func redactOpenAIRequestMediaContent(content *dto.MediaContent) bool {
	if content == nil {
		return false
	}

	changed := false
	if redacted, fieldChanged := RedactSensitiveInfoText(content.Text); fieldChanged {
		content.Text = redacted
		changed = true
	}
	if redacted, fieldChanged := redactSensitiveInfoRawJSON(content.CacheControl, nil); fieldChanged {
		content.CacheControl = redacted
		changed = true
	}
	if redacted, fieldChanged := redactOpenAIRequestTypedMediaValue(content.ImageUrl, dto.ContentTypeImageURL, "image_url"); fieldChanged {
		content.ImageUrl = redacted
		changed = true
	}
	if redacted, fieldChanged := redactOpenAIRequestTypedMediaValue(content.InputAudio, dto.ContentTypeInputAudio, "input_audio"); fieldChanged {
		content.InputAudio = redacted
		changed = true
	}
	if redacted, fieldChanged := redactOpenAIRequestTypedMediaValue(content.File, dto.ContentTypeFile, "file"); fieldChanged {
		content.File = redacted
		changed = true
	}
	if redacted, fieldChanged := redactOpenAIRequestTypedMediaValue(content.VideoUrl, dto.ContentTypeVideoUrl, "video_url"); fieldChanged {
		content.VideoUrl = redacted
		changed = true
	}
	return changed
}

func redactOpenAIRequestTypedMediaValue(value any, contentType, mediaField string) (any, bool) {
	switch media := value.(type) {
	case nil:
		return nil, false
	case map[string]any:
		return redactOpenAIRequestContentAnyWithContext(media, contentType, mediaField)
	case *dto.MessageImageUrl:
		changed := false
		if redacted, fieldChanged := RedactSensitiveInfoText(media.Detail); fieldChanged {
			media.Detail = redacted
			changed = true
		}
		if redacted, fieldChanged := RedactSensitiveInfoText(media.MimeType); fieldChanged {
			media.MimeType = redacted
			changed = true
		}
		return media, changed
	case dto.MessageImageUrl:
		redacted := media
		changed := false
		if text, fieldChanged := RedactSensitiveInfoText(redacted.Detail); fieldChanged {
			redacted.Detail = text
			changed = true
		}
		if text, fieldChanged := RedactSensitiveInfoText(redacted.MimeType); fieldChanged {
			redacted.MimeType = text
			changed = true
		}
		return redacted, changed
	case *dto.MessageInputAudio:
		if redacted, fieldChanged := RedactSensitiveInfoText(media.Format); fieldChanged {
			media.Format = redacted
			return media, true
		}
		return media, false
	case dto.MessageInputAudio:
		redacted := media
		if text, fieldChanged := RedactSensitiveInfoText(redacted.Format); fieldChanged {
			redacted.Format = text
			return redacted, true
		}
		return redacted, false
	case *dto.MessageFile:
		if redacted, fieldChanged := RedactSensitiveInfoText(media.FileName); fieldChanged {
			media.FileName = redacted
			return media, true
		}
		return media, false
	case dto.MessageFile:
		redacted := media
		if text, fieldChanged := RedactSensitiveInfoText(redacted.FileName); fieldChanged {
			redacted.FileName = text
			return redacted, true
		}
		return redacted, false
	default:
		return value, false
	}
}

func redactOpenAIRequestContentAny(v any) (any, bool) {
	return redactOpenAIRequestContentAnyWithContext(v, "", "")
}

func redactOpenAIRequestContentAnyWithContext(v any, contentType, mediaField string) (any, bool) {
	switch value := v.(type) {
	case string:
		return RedactSensitiveInfoText(value)
	case []string:
		redactedSlice := make([]string, len(value))
		changed := false
		for i, item := range value {
			redactedItem, itemChanged := RedactSensitiveInfoText(item)
			redactedSlice[i] = redactedItem
			changed = changed || itemChanged
		}
		return redactedSlice, changed
	case []any:
		redactedSlice := make([]any, len(value))
		changed := false
		for i, item := range value {
			redactedItem, itemChanged := redactOpenAIRequestContentAnyWithContext(item, contentType, mediaField)
			redactedSlice[i] = redactedItem
			changed = changed || itemChanged
		}
		return redactedSlice, changed
	case map[string]any:
		redactedMap := make(map[string]any, len(value))
		changed := false
		currentContentType := contentType
		if itemType, ok := value["type"].(string); ok {
			currentContentType = itemType
		}
		for key, item := range value {
			if shouldPreserveOpenAIContentMediaField(currentContentType, mediaField, key) || shouldPreserveOpenAIOpaqueField(value, key) {
				redactedMap[key] = item
				continue
			}
			childMediaField := mediaField
			if isOpenAIContentMediaContainer(currentContentType, key) {
				childMediaField = key
			} else if mediaField != "" {
				childMediaField = ""
			}
			redactedItem, itemChanged := redactOpenAIRequestContentAnyWithContext(item, currentContentType, childMediaField)
			redactedMap[key] = redactedItem
			changed = changed || itemChanged
		}
		return redactedMap, changed
	default:
		return v, false
	}
}

func isOpenAIContentMediaContainer(contentType, key string) bool {
	switch contentType {
	case dto.ContentTypeImageURL:
		return key == "image_url"
	case dto.ContentTypeInputAudio:
		return key == "input_audio"
	case dto.ContentTypeFile:
		return key == "file"
	case dto.ContentTypeVideoUrl:
		return key == "video_url"
	default:
		return false
	}
}

func shouldPreserveOpenAIContentMediaField(contentType, mediaField, key string) bool {
	switch contentType {
	case dto.ContentTypeImageURL:
		return mediaField == "image_url" && key == "url"
	case dto.ContentTypeInputAudio:
		return mediaField == "input_audio" && key == "data"
	case dto.ContentTypeFile:
		return mediaField == "file" && (key == "file_data" || key == "file_id")
	case dto.ContentTypeVideoUrl:
		return mediaField == "video_url" && key == "url"
	default:
		return false
	}
}

func redactOpenAIRequestRawJSON(raw []byte) ([]byte, bool) {
	if len(raw) == 0 {
		return raw, false
	}

	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return raw, false
	}

	redacted, changed := redactOpenAIRequestAny(value)
	if !changed {
		return raw, false
	}

	data, err := common.Marshal(redacted)
	if err != nil {
		return raw, false
	}

	return data, true
}

func redactOpenAIRequestAny(v any) (any, bool) {
	return redactSensitiveInfoAnyWithPreserve(v, shouldPreserveOpenAIOpaqueField)
}

func redactSensitiveInfoRawJSON(raw []byte, preserveField func(map[string]any, string) bool) ([]byte, bool) {
	if len(raw) == 0 {
		return raw, false
	}

	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return raw, false
	}

	redacted, changed := redactSensitiveInfoAnyWithPreserve(value, preserveField)
	if !changed {
		return raw, false
	}

	data, err := common.Marshal(redacted)
	if err != nil {
		return raw, false
	}
	return data, true
}

func redactSensitiveInfoRawJSONWithPreservePath(raw []byte, preserveField func([]map[string]any, map[string]any, string, any) bool) ([]byte, bool) {
	if len(raw) == 0 {
		return raw, false
	}

	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return raw, false
	}

	redacted, changed := redactSensitiveInfoAnyWithPreservePath(value, preserveField, nil)
	if !changed {
		return raw, false
	}

	data, err := common.Marshal(redacted)
	if err != nil {
		return raw, false
	}
	return data, true
}

func redactSensitiveInfoAnyWithPreserve(v any, preserveField func(map[string]any, string) bool) (any, bool) {
	switch value := v.(type) {
	case string:
		return RedactSensitiveInfoText(value)
	case []string:
		redactedSlice := make([]string, len(value))
		changed := false
		for i, item := range value {
			redactedItem, itemChanged := RedactSensitiveInfoText(item)
			redactedSlice[i] = redactedItem
			changed = changed || itemChanged
		}
		return redactedSlice, changed
	case []any:
		redactedSlice := make([]any, len(value))
		changed := false
		for i, item := range value {
			redactedItem, itemChanged := redactSensitiveInfoAnyWithPreserve(item, preserveField)
			redactedSlice[i] = redactedItem
			changed = changed || itemChanged
		}
		return redactedSlice, changed
	case map[string]any:
		redactedMap := make(map[string]any, len(value))
		changed := false
		for key, item := range value {
			if preserveField != nil && preserveField(value, key) {
				redactedMap[key] = item
				continue
			}
			redactedItem, itemChanged := redactSensitiveInfoAnyWithPreserve(item, preserveField)
			redactedMap[key] = redactedItem
			changed = changed || itemChanged
		}
		return redactedMap, changed
	default:
		return v, false
	}
}

func redactSensitiveInfoAnyWithPreservePath(v any, preserveField func([]map[string]any, map[string]any, string, any) bool, parents []map[string]any) (any, bool) {
	switch value := v.(type) {
	case string:
		return RedactSensitiveInfoText(value)
	case []string:
		redactedSlice := make([]string, len(value))
		changed := false
		for i, item := range value {
			redactedItem, itemChanged := RedactSensitiveInfoText(item)
			redactedSlice[i] = redactedItem
			changed = changed || itemChanged
		}
		return redactedSlice, changed
	case []any:
		redactedSlice := make([]any, len(value))
		changed := false
		for i, item := range value {
			redactedItem, itemChanged := redactSensitiveInfoAnyWithPreservePath(item, preserveField, parents)
			redactedSlice[i] = redactedItem
			changed = changed || itemChanged
		}
		return redactedSlice, changed
	case map[string]any:
		redactedMap := make(map[string]any, len(value))
		changed := false
		childParents := append(parents, value)
		for key, item := range value {
			if preserveField != nil && preserveField(parents, value, key, item) {
				redactedMap[key] = item
				continue
			}
			redactedItem, itemChanged := redactSensitiveInfoAnyWithPreservePath(item, preserveField, childParents)
			redactedMap[key] = redactedItem
			changed = changed || itemChanged
		}
		return redactedMap, changed
	default:
		return v, false
	}
}

func shouldPreserveOpenAIResponsesInputFieldPath(parents []map[string]any, item map[string]any, key string, value any) bool {
	if shouldPreserveOpenAIOpaqueField(item, key) {
		return true
	}
	contentType, _ := item["type"].(string)
	if contentType == "input_image" && key == "image_url" {
		_, isString := value.(string)
		return isString
	}
	if contentType == "input_file" && (key == "file_url" || key == "file_data" || key == "file_id") {
		_, isString := value.(string)
		return isString
	}

	return false
}

func shouldPreserveOpenAIOpaqueField(item map[string]any, key string) bool {
	if key != "encrypted_content" {
		return false
	}
	_, isString := item[key].(string)
	return isString
}

func shouldPreserveGeminiOpaqueField(item map[string]any, key string) bool {
	if key == "thoughtSignature" || key == "thought_signature" {
		_, isString := item[key].(string)
		return isString
	}

	if key == "data" {
		if _, isString := item[key].(string); !isString {
			return false
		}
		if _, ok := item["mimeType"].(string); ok {
			return true
		}
		if _, ok := item["mime_type"].(string); ok {
			return true
		}
	}

	if key == "fileUri" || key == "file_uri" {
		_, isString := item[key].(string)
		return isString
	}

	return false
}

func redactGeminiAny(v any) (any, bool) {
	return redactSensitiveInfoAnyWithPreserve(v, shouldPreserveGeminiOpaqueField)
}

func redactGeminiRawJSON(raw []byte) ([]byte, bool) {
	return redactSensitiveInfoRawJSON(raw, shouldPreserveGeminiOpaqueField)
}

func shouldPreserveImageRequestExtraField(key string) bool {
	switch strings.ToLower(key) {
	case "image", "images", "mask", "file", "files", "audio", "ref_audio", "input_audio", "video", "videos":
		return true
	default:
		return false
	}
}

func redactClaudeContent(content any) (any, bool) {
	switch value := content.(type) {
	case nil:
		return nil, false
	case string:
		return RedactSensitiveInfoText(value)
	case []dto.ClaudeMediaMessage:
		redacted := make([]dto.ClaudeMediaMessage, len(value))
		copy(redacted, value)
		changed := false
		for i := range redacted {
			if redactClaudeMediaMessage(&redacted[i]) {
				changed = true
			}
		}
		return redacted, changed
	case []any:
		redacted := make([]any, len(value))
		changed := false
		for i, item := range value {
			if media, ok := item.(dto.ClaudeMediaMessage); ok {
				if redactClaudeMediaMessage(&media) {
					changed = true
				}
				redacted[i] = media
				continue
			}
			redactedItem, itemChanged := redactClaudeContentAny(item)
			redacted[i] = redactedItem
			changed = changed || itemChanged
		}
		return redacted, changed
	default:
		return redactClaudeContentAny(value)
	}
}

func redactClaudeContentAny(v any) (any, bool) {
	return redactClaudeContentAnyWithContext(v, "", false)
}

func redactClaudeContentAnyWithContext(v any, contentType string, inSource bool) (any, bool) {
	switch value := v.(type) {
	case string:
		return RedactSensitiveInfoText(value)
	case []string:
		redactedSlice := make([]string, len(value))
		changed := false
		for i, item := range value {
			redactedItem, itemChanged := RedactSensitiveInfoText(item)
			redactedSlice[i] = redactedItem
			changed = changed || itemChanged
		}
		return redactedSlice, changed
	case []any:
		redactedSlice := make([]any, len(value))
		changed := false
		for i, item := range value {
			redactedItem, itemChanged := redactClaudeContentAnyWithContext(item, contentType, inSource)
			redactedSlice[i] = redactedItem
			changed = changed || itemChanged
		}
		return redactedSlice, changed
	case map[string]any:
		redactedMap := make(map[string]any, len(value))
		changed := false
		currentContentType := contentType
		if itemType, ok := value["type"].(string); ok && !inSource {
			currentContentType = itemType
		}
		for key, item := range value {
			if shouldPreserveClaudeOpaqueContentField(currentContentType, inSource, key) {
				redactedMap[key] = item
				continue
			}
			childInSource := false
			if isClaudeMediaContentType(currentContentType) && key == "source" {
				childInSource = true
			}
			redactedItem, itemChanged := redactClaudeContentAnyWithContext(item, currentContentType, childInSource)
			redactedMap[key] = redactedItem
			changed = changed || itemChanged
		}
		return redactedMap, changed
	default:
		return v, false
	}
}

func redactClaudeMediaMessage(message *dto.ClaudeMediaMessage) bool {
	if message == nil {
		return false
	}

	changed := false
	if message.Text != nil {
		if redacted, fieldChanged := RedactSensitiveInfoText(*message.Text); fieldChanged {
			*message.Text = redacted
			changed = true
		}
	}
	if message.PartialJson != nil {
		if redacted, fieldChanged := RedactSensitiveInfoText(*message.PartialJson); fieldChanged {
			*message.PartialJson = redacted
			changed = true
		}
	}
	if redacted, fieldChanged := RedactSensitiveInfoText(message.Delta); fieldChanged {
		message.Delta = redacted
		changed = true
	}
	if redacted, fieldChanged := RedactSensitiveInfoAny(message.Input); fieldChanged {
		message.Input = redacted
		changed = true
	}
	if redacted, fieldChanged := redactClaudeContent(message.Content); fieldChanged {
		message.Content = redacted
		changed = true
	}
	if redacted, fieldChanged := redactSensitiveInfoRawJSON(message.CacheControl, nil); fieldChanged {
		message.CacheControl = redacted
		changed = true
	}
	return changed
}

func isClaudeMediaContentType(contentType string) bool {
	switch contentType {
	case "image", "document", "audio", "video":
		return true
	default:
		return false
	}
}

func shouldPreserveClaudeMediaSourceLeaf(contentType string, inSource bool, key string) bool {
	return inSource && isClaudeMediaContentType(contentType) && (key == "data" || key == "url")
}

func shouldPreserveClaudeOpaqueContentField(contentType string, inSource bool, key string) bool {
	if shouldPreserveClaudeMediaSourceLeaf(contentType, inSource, key) {
		return true
	}
	if key == "signature" {
		return true
	}
	if contentType == "thinking" && key == "thinking" {
		return true
	}
	return contentType == "redacted_thinking" && key == "data"
}

func redactGeminiGenerationConfig(config *dto.GeminiChatGenerationConfig) bool {
	if config == nil {
		return false
	}

	changed := false
	for i, stop := range config.StopSequences {
		if redacted, fieldChanged := RedactSensitiveInfoText(stop); fieldChanged {
			config.StopSequences[i] = redacted
			changed = true
		}
	}
	if redacted, fieldChanged := RedactSensitiveInfoAny(config.ResponseSchema); fieldChanged {
		config.ResponseSchema = redacted
		changed = true
	}
	for _, field := range []*jsonRawMessageAlias{
		(*jsonRawMessageAlias)(&config.ResponseJsonSchema),
		(*jsonRawMessageAlias)(&config.SpeechConfig),
		(*jsonRawMessageAlias)(&config.ImageConfig),
	} {
		if redacted, fieldChanged := redactSensitiveInfoRawJSON(*field, nil); fieldChanged {
			*field = redacted
			changed = true
		}
	}
	return changed
}

func redactGeminiChatContent(content *dto.GeminiChatContent) bool {
	if content == nil {
		return false
	}

	changed := false
	for i := range content.Parts {
		if redactGeminiPart(&content.Parts[i]) {
			changed = true
		}
	}
	return changed
}

func redactGeminiPart(part *dto.GeminiPart) bool {
	if part == nil {
		return false
	}

	changed := false
	if redacted, fieldChanged := RedactSensitiveInfoText(part.Text); fieldChanged {
		part.Text = redacted
		changed = true
	}
	if part.FunctionCall != nil {
		if redacted, fieldChanged := redactGeminiAny(part.FunctionCall.Arguments); fieldChanged {
			part.FunctionCall.Arguments = redacted
			changed = true
		}
	}
	if part.FunctionResponse != nil {
		if redacted, fieldChanged := redactGeminiAny(part.FunctionResponse.Response); fieldChanged {
			if response, ok := redacted.(map[string]any); ok {
				part.FunctionResponse.Response = response
				changed = true
			}
		}
		if redacted, fieldChanged := redactGeminiRawJSON(part.FunctionResponse.Parts); fieldChanged {
			part.FunctionResponse.Parts = redacted
			changed = true
		}
	}
	if part.ExecutableCode != nil {
		if redacted, fieldChanged := RedactSensitiveInfoText(part.ExecutableCode.Code); fieldChanged {
			part.ExecutableCode.Code = redacted
			changed = true
		}
	}
	if part.CodeExecutionResult != nil {
		if redacted, fieldChanged := RedactSensitiveInfoText(part.CodeExecutionResult.Output); fieldChanged {
			part.CodeExecutionResult.Output = redacted
			changed = true
		}
	}
	return changed
}

func redactOpenAIRequestToolParameters(parameters any) (any, bool) {
	switch value := parameters.(type) {
	case nil:
		return nil, false
	case []byte:
		redacted, changed := redactOpenAIRequestRawJSON(value)
		if !changed {
			return parameters, false
		}
		return redacted, true
	default:
		return redactOpenAIRequestAny(parameters)
	}
}
