package service

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func TestSensitiveInfoRedact(t *testing.T) {
	input := map[string]any{
		"api_key":            "api_key: sk-testsecretvalue1234567890",
		"bearer_token":       "Authorization: Bearer abcdefghijklmnopqrstuvwx123456",
		"wallet_private_key": "wallet private key: 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"mnemonic_phrase":    "mnemonic: alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu",
		"nested": []any{
			"x-api-key: sk-anothersecretvalue1234567890",
			[]string{"Bearer abcdefghijklmnopqrstuvwx987654", "safe"},
		},
	}

	redactedAny, changed := RedactSensitiveInfoAny(input)
	if !changed {
		t.Fatal("expected changed to be true")
	}

	redacted, ok := redactedAny.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", redactedAny)
	}

	joined := strings.Join([]string{
		redacted["api_key"].(string),
		redacted["bearer_token"].(string),
		redacted["wallet_private_key"].(string),
		redacted["mnemonic_phrase"].(string),
		redacted["nested"].([]any)[0].(string),
		redacted["nested"].([]any)[1].([]string)[0],
	}, "\n")

	for _, secret := range []string{
		"sk-testsecretvalue1234567890",
		"abcdefghijklmnopqrstuvwx123456",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu",
		"sk-anothersecretvalue1234567890",
		"abcdefghijklmnopqrstuvwx987654",
	} {
		if strings.Contains(joined, secret) {
			t.Fatalf("raw secret still present: %s", secret)
		}
	}

	for _, placeholder := range []string{
		"[API_KEY_REDACTED]",
		"[BEARER_TOKEN_REDACTED]",
		"[WALLET_PRIVATE_KEY_REDACTED]",
		"[MNEMONIC_PHRASE_REDACTED]",
	} {
		if !strings.Contains(joined, placeholder) {
			t.Fatalf("missing placeholder: %s", placeholder)
		}
	}

	if got := redacted["nested"].([]any)[1].([]string)[1]; got != "safe" {
		t.Fatalf("expected safe string unchanged, got %q", got)
	}
}

func TestSensitiveInfoRedactNoMatch(t *testing.T) {
	input := map[string]any{
		"text":  "plain prose without secrets",
		"list":  []string{"hello world", "another safe line", "[API_KEY_REDACTED]"},
		"inner": []any{"still safe", map[string]any{"note": "normal text", "token": "[BEARER_TOKEN_REDACTED]"}},
	}

	redactedAny, changed := RedactSensitiveInfoAny(input)
	if changed {
		t.Fatal("expected changed to be false")
	}

	if !reflect.DeepEqual(redactedAny, input) {
		t.Fatalf("expected unchanged value, got %#v", redactedAny)
	}

	redactedMap := redactedAny.(map[string]any)
	if got := redactedMap["list"].([]string)[2]; got != "[API_KEY_REDACTED]" {
		t.Fatalf("expected placeholder unchanged, got %q", got)
	}
	if got := redactedMap["inner"].([]any)[1].(map[string]any)["token"]; got != "[BEARER_TOKEN_REDACTED]" {
		t.Fatalf("expected placeholder unchanged, got %#v", got)
	}

	text, textChanged := RedactSensitiveInfoText("plain prose without secrets")
	if textChanged {
		t.Fatal("expected textChanged to be false")
	}
	if text != "plain prose without secrets" {
		t.Fatalf("expected unchanged text, got %q", text)
	}

	placeholderText, placeholderChanged := RedactSensitiveInfoText("[MNEMONIC_PHRASE_REDACTED]")
	if placeholderChanged {
		t.Fatal("expected placeholder text to remain unchanged")
	}
	if placeholderText != "[MNEMONIC_PHRASE_REDACTED]" {
		t.Fatalf("expected placeholder text unchanged, got %q", placeholderText)
	}
}

func TestRedactSensitiveInfoOpenAIRequest(t *testing.T) {
	toolCallsRaw := mustMarshal(t, []map[string]any{
		{
			"id":   "call_1",
			"type": "function",
			"function": map[string]any{
				"name":      "tool-call",
				"arguments": "api_key: sk-toolcallsecretvalue1234567890",
			},
		},
	})
	functionsRaw := mustMarshal(t, []map[string]any{
		{
			"name":        "lookup",
			"description": "function description Bearer abcdefghijklmnopqrstuvwx123456",
			"parameters": map[string]any{
				"type":        "object",
				"description": "parameters wallet private key: 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		},
	})
	functionCallRaw := mustMarshal(t, map[string]any{
		"name":      "lookup",
		"arguments": "mnemonic: alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu",
	})
	toolParametersRaw := mustMarshal(t, map[string]any{
		"type":        "object",
		"description": "tool parameters api_key: sk-toolparamssecretvalue1234567890",
	})
	nilContentToolCallsRaw := mustMarshal(t, []map[string]any{{
		"id":   "call_nil",
		"type": "function",
		"function": map[string]any{
			"name":      "tool-call-nil",
			"arguments": "api_key: sk-nilcontenttoolsecretvalue1234567890",
		},
	}})

	request := dto.GeneralOpenAIRequest{
		Prompt:      "prompt api_key: sk-promptsecretvalue1234567890",
		Prefix:      []string{"prefix safe", "prefix Bearer abcdefghijklmnopqrstuvwx987654"},
		Suffix:      map[string]any{"note": "suffix wallet private key: 0xabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		Input:       []any{"input mnemonic: alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu", "safe"},
		Instruction: "instruction api_key: sk-instructionsecretvalue1234567890",
		Messages: []dto.Message{
			{
				Role:             "user",
				Content:          "message bearer Bearer abcdefghijklmnopqrstuvwx123456",
				ReasoningContent: stringPtr("reasoning api_key: sk-reasoningsecretvalue1234567890"),
				Reasoning:        stringPtr("reasoning wallet private key: 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
				ToolCalls:        toolCallsRaw,
			},
			{
				Role: "assistant",
				Content: []any{
					map[string]any{"type": dto.ContentTypeText, "text": "array content api_key: sk-arraysecretvalue1234567890"},
					map[string]any{"type": "meta", "note": "safe"},
				},
			},
			{
				Role:             "assistant",
				Content:          nil,
				ReasoningContent: stringPtr("nil content reasoning api_key: sk-nilcontentreasoningsecretvalue1234567890"),
				Reasoning:        stringPtr("nil content reasoning wallet private key: 0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
				ToolCalls:        nilContentToolCallsRaw,
			},
		},
		Functions:    functionsRaw,
		FunctionCall: functionCallRaw,
		Tools: []dto.ToolCallRequest{{
			Type: dto.CustomType,
			Function: dto.FunctionRequest{
				Description: "tool description Bearer abcdefghijklmnopqrstuvwx555555",
				Parameters:  toolParametersRaw,
			},
		}},
	}

	if changed := RedactSensitiveInfoOpenAIRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	assertNoSecret := func(label, got string, secrets ...string) {
		t.Helper()
		for _, secret := range secrets {
			if strings.Contains(got, secret) {
				t.Fatalf("%s still contains raw secret %q: %s", label, secret, got)
			}
		}
	}

	if got := request.Prompt.(string); !strings.Contains(got, "[API_KEY_REDACTED]") {
		t.Fatalf("prompt not redacted: %s", got)
	}
	assertNoSecret("prompt", request.Prompt.(string), "sk-promptsecretvalue1234567890")

	prefix := request.Prefix.([]string)
	if !strings.Contains(prefix[1], "[BEARER_TOKEN_REDACTED]") {
		t.Fatalf("prefix not redacted: %v", prefix)
	}
	assertNoSecret("prefix", strings.Join(prefix, "\n"), "abcdefghijklmnopqrstuvwx987654")

	suffix := request.Suffix.(map[string]any)
	assertNoSecret("suffix", suffix["note"].(string), "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	if !strings.Contains(suffix["note"].(string), "[WALLET_PRIVATE_KEY_REDACTED]") {
		t.Fatalf("suffix not redacted: %v", suffix)
	}

	input := request.Input.([]any)
	assertNoSecret("input", input[0].(string), "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu")
	if !strings.Contains(input[0].(string), "[MNEMONIC_PHRASE_REDACTED]") {
		t.Fatalf("input not redacted: %v", input)
	}

	if got := request.Instruction; !strings.Contains(got, "[API_KEY_REDACTED]") {
		t.Fatalf("instruction not redacted: %s", got)
	}

	msg0 := request.Messages[0]
	assertNoSecret("message string content", msg0.Content.(string), "abcdefghijklmnopqrstuvwx123456")
	if !strings.Contains(msg0.Content.(string), "[BEARER_TOKEN_REDACTED]") {
		t.Fatalf("message string content not redacted: %s", msg0.Content.(string))
	}
	if msg0.ReasoningContent == nil || !strings.Contains(*msg0.ReasoningContent, "[API_KEY_REDACTED]") {
		t.Fatalf("reasoning_content not redacted: %#v", msg0.ReasoningContent)
	}
	if msg0.Reasoning == nil || !strings.Contains(*msg0.Reasoning, "[WALLET_PRIVATE_KEY_REDACTED]") {
		t.Fatalf("reasoning not redacted: %#v", msg0.Reasoning)
	}
	assertNoSecret("tool calls", string(msg0.ToolCalls), "sk-toolcallsecretvalue1234567890")
	if !strings.Contains(string(msg0.ToolCalls), "[API_KEY_REDACTED]") {
		t.Fatalf("tool calls not redacted: %s", string(msg0.ToolCalls))
	}

	msg1 := request.Messages[1]
	content := msg1.Content.([]any)
	firstItem := content[0].(map[string]any)
	if !strings.Contains(firstItem["text"].(string), "[API_KEY_REDACTED]") {
		t.Fatalf("array/map content not redacted: %#v", content)
	}
	assertNoSecret("array/map content", firstItem["text"].(string), "sk-arraysecretvalue1234567890")
	if got := content[1].(map[string]any)["note"].(string); got != "safe" {
		t.Fatalf("safe nested content changed: %v", got)
	}

	nilMsg := request.Messages[2]
	if nilMsg.Content != nil {
		t.Fatalf("nil-content message unexpectedly changed content: %#v", nilMsg.Content)
	}
	if nilMsg.ReasoningContent == nil || !strings.Contains(*nilMsg.ReasoningContent, "[API_KEY_REDACTED]") {
		t.Fatalf("nil-content reasoning_content not redacted: %#v", nilMsg.ReasoningContent)
	}
	if nilMsg.Reasoning == nil || !strings.Contains(*nilMsg.Reasoning, "[WALLET_PRIVATE_KEY_REDACTED]") {
		t.Fatalf("nil-content reasoning not redacted: %#v", nilMsg.Reasoning)
	}
	assertNoSecret("nil-content tool calls", string(nilMsg.ToolCalls), "sk-nilcontenttoolsecretvalue1234567890")
	if !strings.Contains(string(nilMsg.ToolCalls), "[API_KEY_REDACTED]") {
		t.Fatalf("nil-content tool calls not redacted: %s", string(nilMsg.ToolCalls))
	}

	assertNoSecret("functions raw json", string(request.Functions), "abcdefghijklmnopqrstuvwx123456", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if !strings.Contains(string(request.Functions), "[BEARER_TOKEN_REDACTED]") || !strings.Contains(string(request.Functions), "[WALLET_PRIVATE_KEY_REDACTED]") {
		t.Fatalf("functions raw json not redacted: %s", string(request.Functions))
	}
	assertNoSecret("function_call raw json", string(request.FunctionCall), "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu")
	if !strings.Contains(string(request.FunctionCall), "[MNEMONIC_PHRASE_REDACTED]") {
		t.Fatalf("function_call raw json not redacted: %s", string(request.FunctionCall))
	}

	if got := request.Tools[0].Function.Description; !strings.Contains(got, "[BEARER_TOKEN_REDACTED]") {
		t.Fatalf("tool description not redacted: %s", got)
	}
	if raw, ok := request.Tools[0].Function.Parameters.([]byte); !ok {
		t.Fatalf("tool parameters type changed: %T", request.Tools[0].Function.Parameters)
	} else {
		assertNoSecret("tool parameters raw json", string(raw), "sk-toolparamssecretvalue1234567890")
		if !strings.Contains(string(raw), "[API_KEY_REDACTED]") {
			t.Fatalf("tool parameters raw json not redacted: %s", string(raw))
		}
	}
}

func TestRedactSensitiveInfoOpenAIRequestAdditionalFields(t *testing.T) {
	request := dto.GeneralOpenAIRequest{
		ReasoningEffort: "effort api_key: sk-effortsecretvalue1234567890",
		Size:            "size Bearer abcdefghijklmnopqrstuvwx111111",
		Stop:            []any{"stop api_key: sk-stopsecretvalue1234567890"},
		ToolChoice:      map[string]any{"note": "tool choice api_key: sk-toolchoicesecret123456"},
		PromptCacheKey:  "cache key api_key: sk-cachekeysecret1234567890",
		ResponseFormat: &dto.ResponseFormat{
			Type:       "json_schema",
			JsonSchema: mustMarshal(t, map[string]any{"description": "schema api_key: sk-jsonschemasecret123456"}),
		},
		WebSearchOptions: &dto.WebSearchOptions{
			SearchContextSize: "context api_key: sk-searchcontextsecret123456",
			UserLocation:      mustMarshal(t, map[string]any{"city": "location api_key: sk-userlocationsecret123456"}),
		},
		Verbosity:              mustMarshal(t, "verbosity api_key: sk-verbositysecret123456"),
		EncodingFormat:         mustMarshal(t, "encoding api_key: sk-encodingsecret123456"),
		User:                   mustMarshal(t, "user api_key: sk-usersecretvalue1234567890"),
		ServiceTier:            mustMarshal(t, "tier api_key: sk-servicetiersecret123456"),
		Modalities:             mustMarshal(t, []any{"text", "modality api_key: sk-modalitiessecret123456"}),
		Audio:                  mustMarshal(t, map[string]any{"voice": "audio api_key: sk-audiosecretvalue123456"}),
		SafetyIdentifier:       mustMarshal(t, "safety api_key: sk-safetysecretvalue123456"),
		Store:                  mustMarshal(t, map[string]any{"enabled": true, "note": "store api_key: sk-storesecretvalue123456"}),
		PromptCacheRetention:   mustMarshal(t, "retention api_key: sk-retentionsecret123456"),
		LogitBias:              mustMarshal(t, map[string]any{"42": 1, "note": "bias api_key: sk-logitbiassecret123456"}),
		Metadata:               mustMarshal(t, map[string]any{"note": "metadata api_key: sk-metadatasecret123456"}),
		Prediction:             mustMarshal(t, map[string]any{"content": "prediction api_key: sk-predictionsecret123456"}),
		ExtraBody:              mustMarshal(t, map[string]any{"note": "extra api_key: sk-extrabodysecret123456"}),
		SearchParameters:       mustMarshal(t, map[string]any{"query": "search api_key: sk-searchparamssecret123456"}),
		Usage:                  mustMarshal(t, map[string]any{"note": "usage api_key: sk-usagesecretvalue123456"}),
		Reasoning:              mustMarshal(t, map[string]any{"summary": "reasoning api_key: sk-reasoningrawsecret123456"}),
		VlHighResolutionImages: mustMarshal(t, []any{"image api_key: sk-vlsecretvalue123456789"}),
		EnableThinking:         mustMarshal(t, "enable thinking api_key: sk-enablethinkingsecret123"),
		ChatTemplateKwargs:     mustMarshal(t, map[string]any{"note": "template api_key: sk-templatekwargssecret123"}),
		EnableSearch:           mustMarshal(t, false),
		Think:                  mustMarshal(t, "think api_key: sk-thinksecretvalue123456"),
		WebSearch:              mustMarshal(t, map[string]any{"query": "web search api_key: sk-websearchsecret123456"}),
		THINKING:               mustMarshal(t, map[string]any{"note": "thinking api_key: sk-thinkingsecret123456"}),
		SearchDomainFilter:     mustMarshal(t, []any{"domain api_key: sk-domainfiltersecret123"}),
		SearchRecencyFilter:    mustMarshal(t, "recency api_key: sk-recencyfiltersecret123"),
		SearchMode:             mustMarshal(t, "mode api_key: sk-searchmodesecret123456"),
		ReasoningSplit:         mustMarshal(t, "split api_key: sk-reasoningsplitsecret123"),
		Tools: []dto.ToolCallRequest{{
			Type: dto.CustomType,
			Function: dto.FunctionRequest{
				Description: "tool desc api_key: sk-tooldescsecret123456",
				Arguments:   "tool args api_key: sk-toolargssecret123456",
				Parameters: map[string]any{
					"description": "tool params api_key: sk-toolparamsadditionalsecret123",
				},
			},
			Custom: mustMarshal(t, map[string]any{"note": "custom api_key: sk-toolcustomsecret123456"}),
		}},
	}

	if changed := RedactSensitiveInfoOpenAIRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	body := mustMarshalString(t, request)
	assertContains(t, body, "[API_KEY_REDACTED]", "[BEARER_TOKEN_REDACTED]", `"enabled":true`, `"42":1`, `"enable_search":false`)
	assertNotContains(t, body,
		"sk-effortsecretvalue1234567890",
		"abcdefghijklmnopqrstuvwx111111",
		"sk-stopsecretvalue1234567890",
		"sk-toolchoicesecret123456",
		"sk-cachekeysecret1234567890",
		"sk-jsonschemasecret123456",
		"sk-userlocationsecret123456",
		"sk-verbositysecret123456",
		"sk-encodingsecret123456",
		"sk-usersecretvalue1234567890",
		"sk-servicetiersecret123456",
		"sk-modalitiessecret123456",
		"sk-audiosecretvalue123456",
		"sk-safetysecretvalue123456",
		"sk-storesecretvalue123456",
		"sk-retentionsecret123456",
		"sk-logitbiassecret123456",
		"sk-metadatasecret123456",
		"sk-predictionsecret123456",
		"sk-extrabodysecret123456",
		"sk-searchparamssecret123456",
		"sk-usagesecretvalue123456",
		"sk-reasoningrawsecret123456",
		"sk-vlsecretvalue123456789",
		"sk-enablethinkingsecret123",
		"sk-templatekwargssecret123",
		"sk-thinksecretvalue123456",
		"sk-websearchsecret123456",
		"sk-thinkingsecret123456",
		"sk-domainfiltersecret123",
		"sk-recencyfiltersecret123",
		"sk-searchmodesecret123456",
		"sk-reasoningsplitsecret123",
		"sk-tooldescsecret123456",
		"sk-toolargssecret123456",
		"sk-toolparamsadditionalsecret123",
		"sk-toolcustomsecret123456",
	)
}

func TestRedactSensitiveInfoOpenAIRequestMediaOutOfScope(t *testing.T) {
	// 第一版只脱敏文本；图像/音频/文件/视频数据保持原样，不做误改。
	request := dto.GeneralOpenAIRequest{
		Messages: []dto.Message{{
			Role: "user",
			Content: []dto.MediaContent{
				{Type: dto.ContentTypeText, Text: "media text api_key: sk-mediasecretvalue1234567890"},
				{Type: dto.ContentTypeImageURL, ImageUrl: &dto.MessageImageUrl{Url: "https://example.com/image.png?token=sk-media-image-secret", Detail: "high"}},
				{Type: dto.ContentTypeInputAudio, InputAudio: &dto.MessageInputAudio{Data: "audio-base64-sk-media-audio-secret", Format: "wav"}},
				{Type: dto.ContentTypeFile, File: &dto.MessageFile{FileName: "report.txt", FileData: "file-data-sk-media-file-secret", FileId: "file-1"}},
				{Type: dto.ContentTypeVideoUrl, VideoUrl: &dto.MessageVideoUrl{Url: "https://example.com/video.mp4?token=sk-media-video-secret"}},
			},
		}},
	}

	if changed := RedactSensitiveInfoOpenAIRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	content := request.Messages[0].Content.([]dto.MediaContent)
	if got := content[0].Text; !strings.Contains(got, "[API_KEY_REDACTED]") {
		t.Fatalf("text item not redacted: %s", got)
	}
	if got := content[1].ImageUrl.(*dto.MessageImageUrl).Url; got != "https://example.com/image.png?token=sk-media-image-secret" {
		t.Fatalf("image url changed: %s", got)
	}
	if got := content[2].InputAudio.(*dto.MessageInputAudio).Data; got != "audio-base64-sk-media-audio-secret" {
		t.Fatalf("audio data changed: %s", got)
	}
	if got := content[3].File.(*dto.MessageFile).FileData; got != "file-data-sk-media-file-secret" {
		t.Fatalf("file data changed: %s", got)
	}
	if got := content[4].VideoUrl.(*dto.MessageVideoUrl).Url; got != "https://example.com/video.mp4?token=sk-media-video-secret" {
		t.Fatalf("video url changed: %s", got)
	}
}

func TestRedactSensitiveInfoOpenAIRequestTypedMediaContent(t *testing.T) {
	request := dto.GeneralOpenAIRequest{
		Messages: []dto.Message{{
			Role: "user",
			Content: []dto.MediaContent{
				{Type: dto.ContentTypeText, Text: "typed text api_key: sk-typedtextsecret1234567890"},
				{Type: dto.ContentTypeText, CacheControl: mustMarshal(t, map[string]any{"note": "cache api_key: sk-typedcachesecret123456"})},
				{Type: dto.ContentTypeImageURL, ImageUrl: map[string]any{"url": "https://example.com/typed.png?token=sk-typed-image-payload", "detail": "detail api_key: sk-typedimagedetailsecret123", "note": "image note api_key: sk-typedimagenotesecret123"}},
				{Type: dto.ContentTypeInputAudio, InputAudio: map[string]any{"data": "typed-audio-base64-sk-payload", "format": "format api_key: sk-typedaudioformatsecret123", "note": "audio note api_key: sk-typedaudionotesecret123"}},
				{Type: dto.ContentTypeFile, File: map[string]any{"file_data": "typed-file-data-sk-payload", "file_id": "file-sk-typed-payload", "file_name": "file name api_key: sk-typedfilenamesecret123", "note": "file note api_key: sk-typedfilenotesecret123"}},
				{Type: dto.ContentTypeVideoUrl, VideoUrl: map[string]any{"url": "https://example.com/typed.mp4?token=sk-typed-video-payload", "note": "video note api_key: sk-typedvideonotesecret123"}},
				{Type: dto.ContentTypeImageURL, ImageUrl: &dto.MessageImageUrl{Url: "https://example.com/ptr.png?token=sk-ptr-image-payload", Detail: "ptr detail api_key: sk-ptrimagedetailsecret123", MimeType: "mime api_key: sk-ptrimagemimesecret123"}},
				{Type: dto.ContentTypeInputAudio, InputAudio: &dto.MessageInputAudio{Data: "ptr-audio-base64-sk-payload", Format: "ptr format api_key: sk-ptraudioformatsecret123"}},
				{Type: dto.ContentTypeFile, File: &dto.MessageFile{FileName: "ptr file api_key: sk-ptrfilenamesecret123", FileData: "ptr-file-data-sk-payload", FileId: "file-sk-ptr-payload"}},
				{Type: dto.ContentTypeVideoUrl, VideoUrl: &dto.MessageVideoUrl{Url: "https://example.com/ptr.mp4?token=sk-ptr-video-payload"}},
			},
		}},
	}

	if changed := RedactSensitiveInfoOpenAIRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	content := request.Messages[0].Content.([]dto.MediaContent)
	assertContains(t, content[0].Text, "[API_KEY_REDACTED]")
	assertContains(t, string(content[1].CacheControl), "[API_KEY_REDACTED]")

	body := mustMarshalString(t, request)
	assertContains(t, body,
		"https://example.com/typed.png?token=sk-typed-image-payload",
		"typed-audio-base64-sk-payload",
		"typed-file-data-sk-payload",
		"file-sk-typed-payload",
		"https://example.com/typed.mp4?token=sk-typed-video-payload",
		"https://example.com/ptr.png?token=sk-ptr-image-payload",
		"ptr-audio-base64-sk-payload",
		"ptr-file-data-sk-payload",
		"file-sk-ptr-payload",
		"https://example.com/ptr.mp4?token=sk-ptr-video-payload",
	)
	assertNotContains(t, body,
		"sk-typedtextsecret1234567890",
		"sk-typedcachesecret123456",
		"sk-typedimagedetailsecret123",
		"sk-typedimagenotesecret123",
		"sk-typedaudioformatsecret123",
		"sk-typedaudionotesecret123",
		"sk-typedfilenamesecret123",
		"sk-typedfilenotesecret123",
		"sk-typedvideonotesecret123",
		"sk-ptrimagedetailsecret123",
		"sk-ptrimagemimesecret123",
		"sk-ptraudioformatsecret123",
		"sk-ptrfilenamesecret123",
	)
}

func TestRedactSensitiveInfoOpenAIRequestMediaOnlyPreservesLeafPayloads(t *testing.T) {
	request := dto.GeneralOpenAIRequest{
		Messages: []dto.Message{{
			Role: "user",
			Content: []any{
				map[string]any{
					"type":      dto.ContentTypeImageURL,
					"image_url": map[string]any{"url": "https://example.com/image.png?token=sk-media-image-secret", "note": "image note api_key: sk-imagenotesecret123456"},
				},
				map[string]any{
					"type":        dto.ContentTypeInputAudio,
					"input_audio": map[string]any{"data": "audio-base64-sk-media-audio-secret", "format": "format api_key: sk-audioformatsecret123456", "note": "audio note api_key: sk-audionotesecret123456"},
				},
				map[string]any{
					"type": dto.ContentTypeFile,
					"file": map[string]any{"file_data": "file-data-sk-media-file-secret", "file_id": "file-sk-media-file-id-secret", "file_name": "file name api_key: sk-filenamesecret123456", "note": "file note api_key: sk-filenotesecret123456"},
				},
				map[string]any{
					"type":      dto.ContentTypeVideoUrl,
					"video_url": map[string]any{"url": "https://example.com/video.mp4?token=sk-media-video-secret", "note": "video note api_key: sk-videonotesecret123456"},
				},
			},
		}},
	}

	if changed := RedactSensitiveInfoOpenAIRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	body := mustMarshalString(t, request)
	assertContains(t, body,
		"https://example.com/image.png?token=sk-media-image-secret",
		"audio-base64-sk-media-audio-secret",
		"file-data-sk-media-file-secret",
		"file-sk-media-file-id-secret",
		"https://example.com/video.mp4?token=sk-media-video-secret",
		"[API_KEY_REDACTED]",
	)
	assertNotContains(t, body,
		"sk-imagenotesecret123456",
		"sk-audioformatsecret123456",
		"sk-audionotesecret123456",
		"sk-filenamesecret123456",
		"sk-filenotesecret123456",
		"sk-videonotesecret123456",
	)
}

func TestRedactSensitiveInfoOpenAIResponsesRequest(t *testing.T) {
	request := dto.OpenAIResponsesRequest{
		Input: mustMarshal(t, []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "input_text", "text": "responses input api_key: sk-responsesinputsecret1234567890"},
				{"type": "input_image", "image_url": "https://example.com/image.png?token=sk-responses-image-secret", "note": "image note api_key: sk-responsesimagenotesecret123456"},
				{"type": "input_file", "file_url": "https://example.com/file.pdf?token=sk-responses-file-secret", "file_data": "file-data-sk-responses-file-data-secret", "file_id": "file-sk-responses-file-id-secret", "note": "file note api_key: sk-responsesfilenotesecret123456"},
			},
		}}),
		Instructions:      mustMarshal(t, "responses instructions Bearer abcdefghijklmnopqrstuvwx123456"),
		Metadata:          mustMarshal(t, map[string]any{"note": "metadata api_key: sk-responsesmetadatasecret123456"}),
		Text:              mustMarshal(t, map[string]any{"format": "text wallet private key: 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}),
		ToolChoice:        mustMarshal(t, map[string]any{"note": "tool choice api_key: sk-responsestoolchoicesecret123"}),
		Tools:             mustMarshal(t, []map[string]any{{"description": "tool Bearer abcdefghijklmnopqrstuvwx654321"}}),
		Prompt:            mustMarshal(t, map[string]any{"id": "prompt api_key: sk-responsespromptsecret123456"}),
		User:              mustMarshal(t, "user api_key: sk-responsesusersecret123456789"),
		Conversation:      mustMarshal(t, "conversation api_key: sk-responsesconversationsecret"),
		ContextManagement: mustMarshal(t, map[string]any{"note": "context api_key: sk-responsescontextsecret123456"}),
		ParallelToolCalls: mustMarshal(t, "parallel api_key: sk-responsesparallelsecret123456"),
		Truncation:        mustMarshal(t, "truncation api_key: sk-responsestruncationsecret"),
		EnableThinking:    mustMarshal(t, "thinking api_key: sk-responsesthinkingsecret123"),
		Preset:            mustMarshal(t, "preset api_key: sk-responsespresetsecret123456"),
		Store:             mustMarshal(t, "store api_key: sk-responsesstoresecret1234567"),
		PromptCacheKey:    mustMarshal(t, "cache api_key: sk-responsescachesecret1234567"),
		PromptCacheRetention: mustMarshal(t, map[string]any{
			"note": "retention api_key: sk-responsesretentionsecret123",
		}),
		SafetyIdentifier: mustMarshal(t, "safety api_key: sk-responsessafetysecret123456"),
		Reasoning:        &dto.Reasoning{Summary: "summary api_key: sk-responsessummarysecret123456"},
	}

	if changed := RedactSensitiveInfoOpenAIResponsesRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	joined := strings.Join([]string{
		string(request.Input), string(request.Instructions), string(request.Metadata), string(request.Text),
		string(request.ToolChoice), string(request.Tools), string(request.Prompt), string(request.User),
		string(request.Conversation), string(request.ContextManagement), string(request.ParallelToolCalls), string(request.Truncation),
		string(request.EnableThinking), string(request.Preset), string(request.Store), string(request.PromptCacheKey),
		string(request.PromptCacheRetention), string(request.SafetyIdentifier), request.Reasoning.Summary,
	}, "\n")
	assertContains(t, joined, "[API_KEY_REDACTED]")
	assertContains(t, joined, "[BEARER_TOKEN_REDACTED]")
	assertContains(t, joined, "[WALLET_PRIVATE_KEY_REDACTED]")
	assertNotContains(t, joined, "sk-responsesinputsecret1234567890", "abcdefghijklmnopqrstuvwx123456", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	assertContains(t, string(request.Input), "https://example.com/image.png?token=sk-responses-image-secret")
	assertContains(t, string(request.Input), "https://example.com/file.pdf?token=sk-responses-file-secret")
	assertContains(t, string(request.Input), "file-data-sk-responses-file-data-secret")
	assertContains(t, string(request.Input), "file-sk-responses-file-id-secret")
	assertNotContains(t, string(request.Input), "sk-responsesimagenotesecret123456", "sk-responsesfilenotesecret123456")
}

func TestRedactSensitiveInfoOpenAIPreservesEncryptedContent(t *testing.T) {
	encryptedContent := "gAAAAABl.sk-opaque-encrypted-content"
	plainSecret := "sk-opaque-secret-value123456"
	request := dto.OpenAIResponsesRequest{
		Input: mustMarshal(t, []map[string]any{{
			"role": "assistant",
			"content": []map[string]any{
				{"type": "reasoning", "encrypted_content": encryptedContent, "note": "reasoning note " + plainSecret},
				{"type": "input_text", "text": "responses input " + plainSecret},
			},
		}}),
		Metadata: mustMarshal(t, map[string]any{
			"encrypted_content": encryptedContent,
			"note":              "metadata " + plainSecret,
		}),
	}

	if changed := RedactSensitiveInfoOpenAIResponsesRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	joined := strings.Join([]string{string(request.Input), string(request.Metadata)}, "\n")
	assertContains(t, joined, encryptedContent, "[API_KEY_REDACTED]")
	assertNotContains(t, joined, "reasoning note "+plainSecret, "responses input "+plainSecret, "metadata "+plainSecret)
}

func TestRedactSensitiveInfoOpenAIRequestPreservesEncryptedContent(t *testing.T) {
	encryptedContent := "gAAAAABl.sk-opaque-encrypted-content"
	plainSecret := "sk-opaque-secret-value123456"
	request := dto.GeneralOpenAIRequest{
		Input: []any{
			map[string]any{"type": "reasoning", "encrypted_content": encryptedContent, "note": "input note " + plainSecret},
			"input " + plainSecret,
		},
		Reasoning: mustMarshal(t, map[string]any{
			"encrypted_content": encryptedContent,
			"note":              "reasoning raw " + plainSecret,
		}),
		ExtraBody: mustMarshal(t, map[string]any{
			"items": []map[string]any{{
				"encrypted_content": encryptedContent,
				"note":              "extra raw " + plainSecret,
			}},
		}),
	}

	if changed := RedactSensitiveInfoOpenAIRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	body := mustMarshalString(t, request)
	assertContains(t, body, encryptedContent, "[API_KEY_REDACTED]")
	assertNotContains(t, body, "input note "+plainSecret, "input "+plainSecret, "reasoning raw "+plainSecret, "extra raw "+plainSecret)
}

func TestRedactSensitiveInfoOpenAIResponsesRawFieldsPreserveEncryptedContent(t *testing.T) {
	secret := "sk-" + strings.Repeat("a", 20)
	encryptedContent := "gAAAAABl." + secret
	request := dto.OpenAIResponsesRequest{
		Tools: mustMarshal(t, []map[string]any{{
			"encrypted_content": encryptedContent,
			"note":              "tool " + secret,
		}}),
		Metadata: mustMarshal(t, map[string]any{
			"encrypted_content": encryptedContent,
			"note":              "metadata " + secret,
		}),
	}

	if changed := RedactSensitiveInfoOpenAIResponsesRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	joined := strings.Join([]string{string(request.Tools), string(request.Metadata)}, "\n")
	assertContains(t, joined, encryptedContent, "[API_KEY_REDACTED]")
	assertNotContains(t, joined, "tool "+secret, "metadata "+secret)
}

func TestRedactSensitiveInfoOpenAIResponsesCompactionPreservesEncryptedContent(t *testing.T) {
	secret := "sk-" + strings.Repeat("b", 20)
	encryptedContent := "gAAAAABl." + secret
	request := dto.OpenAIResponsesCompactionRequest{
		Input: mustMarshal(t, []map[string]any{{
			"type":              "reasoning",
			"encrypted_content": encryptedContent,
			"note":              "input " + secret,
		}}),
		Instructions: mustMarshal(t, map[string]any{
			"encrypted_content": encryptedContent,
			"note":              "instructions " + secret,
		}),
	}

	if changed := RedactSensitiveInfoOpenAIResponsesCompactionRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	joined := strings.Join([]string{string(request.Input), string(request.Instructions)}, "\n")
	assertContains(t, joined, encryptedContent, "[API_KEY_REDACTED]")
	assertNotContains(t, joined, "input "+secret, "instructions "+secret)
}

func TestRedactSensitiveInfoEmbeddingRequest(t *testing.T) {
	request := dto.EmbeddingRequest{
		Input: []any{"embedding api_key: sk-embeddinginputsecret123456", "safe"},
		User:  "embedding user Bearer abcdefghijklmnopqrstuvwx123456",
	}

	if changed := RedactSensitiveInfoEmbeddingRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}
	joined := strings.Join([]string{request.Input.([]any)[0].(string), request.User}, "\n")
	assertContains(t, joined, "[API_KEY_REDACTED]")
	assertContains(t, joined, "[BEARER_TOKEN_REDACTED]")
	assertNotContains(t, joined, "sk-embeddinginputsecret123456", "abcdefghijklmnopqrstuvwx123456")
}

func TestRedactSensitiveInfoRerankRequest(t *testing.T) {
	request := dto.RerankRequest{
		Documents: []any{"doc api_key: sk-rerankdocsecret1234567890", map[string]any{"text": "wallet private key: 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}},
		Query:     "query Bearer abcdefghijklmnopqrstuvwx123456",
	}

	if changed := RedactSensitiveInfoRerankRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}
	joined := strings.Join([]string{request.Documents[0].(string), request.Documents[1].(map[string]any)["text"].(string), request.Query}, "\n")
	assertContains(t, joined, "[API_KEY_REDACTED]")
	assertContains(t, joined, "[BEARER_TOKEN_REDACTED]")
	assertContains(t, joined, "[WALLET_PRIVATE_KEY_REDACTED]")
	assertNotContains(t, joined, "sk-rerankdocsecret1234567890", "abcdefghijklmnopqrstuvwx123456", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
}

func TestRedactSensitiveInfoImageRequest(t *testing.T) {
	request := dto.ImageRequest{
		Model:             "image-model-sk-protocol-unchanged",
		Prompt:            "image prompt api_key: sk-imagepromptsecret1234567890",
		Size:              "size api_key: sk-imagesizesecret123456",
		Quality:           "quality api_key: sk-imagequalitysecret123456",
		ResponseFormat:    "response format api_key: sk-imageresponseformatsecret123",
		Style:             mustMarshal(t, "style Bearer abcdefghijklmnopqrstuvwx123456"),
		User:              mustMarshal(t, "user api_key: sk-imageusersecret123456789"),
		ExtraFields:       mustMarshal(t, map[string]any{"negative_prompt": "wallet private key: 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}),
		Background:        mustMarshal(t, "background api_key: sk-imagebackgroundsecret123"),
		Moderation:        mustMarshal(t, "moderation api_key: sk-imagemoderationsecret123"),
		OutputFormat:      mustMarshal(t, "output format api_key: sk-imageoutputformatsecret123"),
		OutputCompression: mustMarshal(t, map[string]any{"note": "compression api_key: sk-imagecompressionsecret123", "level": 80}),
		PartialImages:     mustMarshal(t, "partial api_key: sk-imagepartialsecret123456"),
		InputFidelity:     mustMarshal(t, "fidelity api_key: sk-imagefidelitysecret123"),
		WatermarkEnabled:  mustMarshal(t, false),
		UserId:            mustMarshal(t, "user id api_key: sk-imageuseridsecret123456"),
		Image:             mustMarshal(t, "image-base64-sk-image-payload-secret"),
		Images:            mustMarshal(t, []string{"image-list-sk-image-payload-secret"}),
		Mask:              mustMarshal(t, "mask-base64-sk-image-mask-secret"),
		Extra: map[string]json.RawMessage{
			"text_note": mustMarshal(t, "extra api_key: sk-imageextrasecret123456"),
			"image":     mustMarshal(t, "extra-image-sk-image-extra-secret"),
		},
	}

	if changed := RedactSensitiveInfoImageRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}
	joined := strings.Join([]string{request.Prompt, request.Size, request.Quality, request.ResponseFormat, string(request.Style), string(request.User), string(request.ExtraFields), string(request.Background), string(request.Moderation), string(request.OutputFormat), string(request.OutputCompression), string(request.PartialImages), string(request.InputFidelity), string(request.UserId), string(request.Extra["text_note"])}, "\n")
	assertContains(t, joined, "[API_KEY_REDACTED]")
	assertContains(t, joined, "[BEARER_TOKEN_REDACTED]")
	assertContains(t, joined, "[WALLET_PRIVATE_KEY_REDACTED]")
	assertNotContains(t, joined, "sk-imagepromptsecret1234567890", "sk-imagesizesecret123456", "sk-imagequalitysecret123456", "sk-imageresponseformatsecret123", "abcdefghijklmnopqrstuvwx123456", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "sk-imagemoderationsecret123", "sk-imageoutputformatsecret123", "sk-imagecompressionsecret123", "sk-imagepartialsecret123456", "sk-imagefidelitysecret123", "sk-imageuseridsecret123456")
	if request.Model != "image-model-sk-protocol-unchanged" {
		t.Fatalf("model changed: %s", request.Model)
	}
	if got := string(request.WatermarkEnabled); got != `false` {
		t.Fatalf("watermark_enabled bool changed: %s", got)
	}
	if got := string(request.Image); got != `"image-base64-sk-image-payload-secret"` {
		t.Fatalf("image payload changed: %s", got)
	}
	if got := string(request.Images); got != `["image-list-sk-image-payload-secret"]` {
		t.Fatalf("images payload changed: %s", got)
	}
	if got := string(request.Mask); got != `"mask-base64-sk-image-mask-secret"` {
		t.Fatalf("mask payload changed: %s", got)
	}
	if got := string(request.Extra["image"]); got != `"extra-image-sk-image-extra-secret"` {
		t.Fatalf("extra image payload changed: %s", got)
	}
}

func TestRedactSensitiveInfoAudioRequest(t *testing.T) {
	request := dto.AudioRequest{
		Model:                   "audio-model-sk-protocol-unchanged",
		Input:                   "audio input api_key: sk-audioinputsecret1234567890",
		Voice:                   "voice Bearer abcdefghijklmnopqrstuvwx123456",
		Instructions:            "audio instructions wallet private key: 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ResponseFormat:          "response api_key: sk-audioresponseformatsecret123",
		StreamFormat:            "stream api_key: sk-audiostreamformatsecret123",
		Metadata:                mustMarshal(t, map[string]any{"note": "metadata api_key: sk-audiometadatasecret123456"}),
		TaskType:                mustMarshal(t, "task api_key: sk-audiotasksecret123456789"),
		Language:                mustMarshal(t, "language api_key: sk-audiolanguagesecret1234"),
		RefText:                 mustMarshal(t, "ref text api_key: sk-audioreftextsecret123456"),
		RefAudio:                mustMarshal(t, "ref-audio-sk-audio-payload-secret"),
		XVectorOnlyMode:         mustMarshal(t, map[string]any{"enabled": true, "note": "xvector api_key: sk-audioxvectorsecret123456"}),
		MaxNewTokens:            mustMarshal(t, 128),
		InitialCodecChunkFrames: mustMarshal(t, "codec frames api_key: sk-audiocodecframessecret123"),
	}

	if changed := RedactSensitiveInfoAudioRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}
	joined := strings.Join([]string{request.Input, request.Voice, request.Instructions, request.ResponseFormat, request.StreamFormat, string(request.Metadata), string(request.TaskType), string(request.Language), string(request.RefText), string(request.XVectorOnlyMode), string(request.InitialCodecChunkFrames)}, "\n")
	assertContains(t, joined, "[API_KEY_REDACTED]")
	assertContains(t, joined, "[BEARER_TOKEN_REDACTED]")
	assertContains(t, joined, "[WALLET_PRIVATE_KEY_REDACTED]")
	assertNotContains(t, joined, "sk-audioinputsecret1234567890", "abcdefghijklmnopqrstuvwx123456", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "sk-audioresponseformatsecret123", "sk-audiostreamformatsecret123", "sk-audioxvectorsecret123456", "sk-audiocodecframessecret123")
	if request.Model != "audio-model-sk-protocol-unchanged" {
		t.Fatalf("model changed: %s", request.Model)
	}
	if got := string(request.RefAudio); got != `"ref-audio-sk-audio-payload-secret"` {
		t.Fatalf("ref audio payload changed: %s", got)
	}
	if got := string(request.MaxNewTokens); got != `128` {
		t.Fatalf("max_new_tokens number changed: %s", got)
	}
}

func TestRedactSensitiveInfoClaudeRequest(t *testing.T) {
	text := "claude media text api_key: sk-claudemediasecret1234567890"
	request := dto.ClaudeRequest{
		Model:         "claude-model-sk-protocol-unchanged",
		Prompt:        "claude prompt api_key: sk-claudepromptsecret123456",
		StopSequences: []string{"stop api_key: sk-claudestopsecret123456"},
		CacheControl:  mustMarshal(t, map[string]any{"note": "cache api_key: sk-claudecachesecret123456", "ttl": 300}),
		McpServers:    mustMarshal(t, []map[string]any{{"name": "mcp api_key: sk-claudemcpsecret123456", "enabled": true}}),
		InferenceGeo:  "us-sk-protocol-unchanged",
		ServiceTier:   "standard-sk-protocol-unchanged",
		Thinking:      &dto.Thinking{Type: "enabled-sk-protocol-unchanged", Display: "summarized-sk-protocol-unchanged"},
		System: []any{
			map[string]any{"type": "text", "text": "system Bearer abcdefghijklmnopqrstuvwx123456"},
			map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "claude-image-sk-claude-image-secret", "url": "https://example.com/claude.png?token=sk-claude-url-secret", "note": "source note api_key: sk-claudesourcenotesecret123456"}},
		},
		Messages: []dto.ClaudeMessage{{Content: []dto.ClaudeMediaMessage{{Type: "text", Text: &text, Delta: "delta api_key: sk-claudedeltasecret123456"}, {Type: "image", Source: &dto.ClaudeMessageSource{Type: "base64", Data: "claude-image-sk-claude-message-image-secret"}}}}},
		Tools:    []any{map[string]any{"description": "tool wallet private key: 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}},
		Metadata: mustMarshal(t, map[string]any{"user_id": "metadata api_key: sk-claudemetadatasecret12345"}),
	}

	if changed := RedactSensitiveInfoClaudeRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}
	joined := mustMarshalString(t, request)
	assertContains(t, joined, "[API_KEY_REDACTED]")
	assertContains(t, joined, "[BEARER_TOKEN_REDACTED]")
	assertContains(t, joined, "[WALLET_PRIVATE_KEY_REDACTED]")
	assertNotContains(t, joined, "sk-claudepromptsecret123456", "sk-claudestopsecret123456", "sk-claudecachesecret123456", "sk-claudemcpsecret123456", "sk-claudesourcenotesecret123456", "sk-claudedeltasecret123456", "abcdefghijklmnopqrstuvwx123456", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	assertContains(t, joined, "claude-model-sk-protocol-unchanged", "us-sk-protocol-unchanged", "standard-sk-protocol-unchanged", "enabled-sk-protocol-unchanged", "summarized-sk-protocol-unchanged")
	assertContains(t, joined, "claude-image-sk-claude-image-secret")
	assertContains(t, joined, "https://example.com/claude.png?token=sk-claude-url-secret")
	assertContains(t, joined, "claude-image-sk-claude-message-image-secret")
}

func TestRedactSensitiveInfoClaudeOpaqueThinkingFields(t *testing.T) {
	thinking := "gAAAAABl.sk-opaqueencryptedthinkingpayload123456"
	signature := "sig.sk-opaquesignaturepayload123456"
	redactedThinkingData := "gAAAAABl.sk-redactedthinkingpayload123456"
	text := "plain text api_key: sk-textpayloadabcdefghijkl"

	request := dto.ClaudeRequest{
		Prompt: "prompt api_key: sk-promptpayloadabcdefghijkl",
		System: []any{
			map[string]any{"type": "thinking", "thinking": thinking, "signature": signature, "note": "note api_key: sk-systemnoteabcdefghijkl"},
			map[string]any{"type": "redacted_thinking", "data": redactedThinkingData, "note": "note api_key: sk-redactednoteabcdefghijkl"},
		},
		Messages: []dto.ClaudeMessage{{Content: []dto.ClaudeMediaMessage{
			{Type: "thinking", Thinking: &thinking, Signature: signature},
			{Type: "text", Text: &text},
		}}},
	}

	if changed := RedactSensitiveInfoClaudeRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	joined := mustMarshalString(t, request)
	assertContains(t, joined, thinking, signature, redactedThinkingData, "[API_KEY_REDACTED]")
	assertNotContains(t, joined, "sk-promptpayloadabcdefghijkl", "sk-textpayloadabcdefghijkl", "sk-systemnoteabcdefghijkl", "sk-redactednoteabcdefghijkl")
}

func TestRedactSensitiveInfoGeminiChatRequest(t *testing.T) {
	request := dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{{Parts: []dto.GeminiPart{
			{Text: "gemini text api_key: sk-geminitextsecret1234567890"},
			{InlineData: &dto.GeminiInlineData{MimeType: "image/png", Data: "gemini-inline-sk-gemini-inline-secret"}},
			{FileData: &dto.GeminiFileData{MimeType: "image/png", FileUri: "gs://bucket/sk-gemini-file-secret"}},
			{FunctionCall: &dto.FunctionCall{FunctionName: "lookup", Arguments: map[string]any{"q": "Bearer abcdefghijklmnopqrstuvwx123456"}}},
			{ThoughtSignature: mustMarshal(t, "thought-signature-sk-opaque-secret")},
			{FunctionResponse: &dto.GeminiFunctionResponse{
				Name:         "lookup-sk-protocol-unchanged",
				Response:     map[string]any{"answer": "function response api_key: sk-geminifunctionresponsesecret123"},
				Parts:        mustMarshal(t, []map[string]any{{"text": "parts api_key: sk-geminipartssecret123456"}}),
				ID:           mustMarshal(t, "function-id-sk-opaque-secret"),
				WillContinue: mustMarshal(t, "will-continue-sk-protocol-unchanged"),
				Scheduling:   mustMarshal(t, map[string]any{"mode": "scheduling-sk-protocol-unchanged"}),
			}},
		}}},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			StopSequences:      []string{"stop api_key: sk-geministopsecret123456"},
			ResponseMimeType:   "application/json-sk-protocol-unchanged",
			ResponseSchema:     map[string]any{"description": "schema api_key: sk-geminiresponseschemasecret123", "type": "object"},
			ResponseJsonSchema: mustMarshal(t, map[string]any{"description": "json schema api_key: sk-geminiresponsejsonschemasecret123"}),
			ResponseModalities: []string{"TEXT-sk-protocol-unchanged"},
			ThinkingConfig:     &dto.GeminiThinkingConfig{ThinkingLevel: "high-sk-protocol-unchanged"},
			SpeechConfig:       mustMarshal(t, map[string]any{"voice": "speech api_key: sk-geminispeechsecret123456", "enabled": true}),
			ImageConfig:        mustMarshal(t, map[string]any{"note": "image config api_key: sk-geminiimageconfigsecret123", "count": 2}),
		},
		SystemInstructions: &dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: "system wallet private key: 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}},
		Tools:              mustMarshal(t, []map[string]any{{"functionDeclarations": []map[string]any{{"description": "tool api_key: sk-geminitoolsecret123456789"}}}}),
		ToolConfig:         &dto.ToolConfig{FunctionCallingConfig: &dto.FunctionCallingConfig{AllowedFunctionNames: []string{"lookup-sk-protocol-unchanged"}}, RetrievalConfig: &dto.RetrievalConfig{LanguageCode: "language api_key: sk-geminilanguagesecret123456"}},
	}

	if changed := RedactSensitiveInfoGeminiChatRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}
	joined := mustMarshalString(t, request)
	assertContains(t, joined, "[API_KEY_REDACTED]")
	assertContains(t, joined, "[BEARER_TOKEN_REDACTED]")
	assertContains(t, joined, "[WALLET_PRIVATE_KEY_REDACTED]")
	assertNotContains(t, joined, "sk-geminitextsecret1234567890", "sk-geminifunctionresponsesecret123", "sk-geminipartssecret123456", "sk-geministopsecret123456", "sk-geminiresponseschemasecret123", "sk-geminiresponsejsonschemasecret123", "sk-geminispeechsecret123456", "sk-geminiimageconfigsecret123", "sk-geminilanguagesecret123456", "abcdefghijklmnopqrstuvwx123456", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	assertContains(t, joined, "gemini-inline-sk-gemini-inline-secret")
	assertContains(t, joined, "gs://bucket/sk-gemini-file-secret")
	assertContains(t, joined, "thought-signature-sk-opaque-secret", "function-id-sk-opaque-secret", "will-continue-sk-protocol-unchanged", "scheduling-sk-protocol-unchanged", "lookup-sk-protocol-unchanged", "application/json-sk-protocol-unchanged", "TEXT-sk-protocol-unchanged", "high-sk-protocol-unchanged")
}

func TestRedactSensitiveInfoGeminiPreservesOpaqueRawFields(t *testing.T) {
	secret := "sk-" + strings.Repeat("c", 20)
	opaquePayload := "gemini-opaque-" + secret
	request := dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{{Parts: []dto.GeminiPart{{
			FunctionCall: &dto.FunctionCall{Arguments: map[string]any{
				"inlineData": map[string]any{"mimeType": "image/png", "data": opaquePayload, "note": "inline " + secret},
			}},
		}, {
			FunctionResponse: &dto.GeminiFunctionResponse{
				Response: map[string]any{
					"fileData": map[string]any{"mimeType": "image/png", "fileUri": "gs://bucket/" + opaquePayload, "note": "file " + secret},
				},
				Parts: mustMarshal(t, []map[string]any{{
					"thoughtSignature": opaquePayload,
					"note":             "parts " + secret,
				}}),
			},
		}}}},
		Tools: mustMarshal(t, []map[string]any{{
			"inlineData":       map[string]any{"mime_type": "image/png", "data": opaquePayload, "note": "tool inline " + secret},
			"thoughtSignature": opaquePayload,
			"note":             "tool " + secret,
		}}),
	}

	if changed := RedactSensitiveInfoGeminiChatRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}

	joined := mustMarshalString(t, request)
	assertContains(t, joined, opaquePayload, "gs://bucket/"+opaquePayload, "[API_KEY_REDACTED]")
	assertNotContains(t, joined, "inline "+secret, "file "+secret, "parts "+secret, "tool inline "+secret, "tool "+secret)
}

func TestRedactSensitiveInfoGeminiEmbeddingRequest(t *testing.T) {
	request := dto.GeminiEmbeddingRequest{
		Content:  dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: "embedding api_key: sk-geminiembeddingsecret123456"}}},
		TaskType: "task Bearer abcdefghijklmnopqrstuvwx123456",
		Title:    "title api_key: sk-geminiembeddingtitlesecret123",
	}

	if changed := RedactSensitiveInfoGeminiEmbeddingRequest(&request); !changed {
		t.Fatal("expected changed to be true")
	}
	joined := mustMarshalString(t, request)
	assertContains(t, joined, "[API_KEY_REDACTED]")
	assertContains(t, joined, "[BEARER_TOKEN_REDACTED]")
	assertNotContains(t, joined, "sk-geminiembeddingsecret123456", "abcdefghijklmnopqrstuvwx123456")
}

func TestRedactSensitiveInfoRequestDispatcher(t *testing.T) {
	tests := []struct {
		name    string
		request dto.Request
	}{
		{name: "openai", request: &dto.GeneralOpenAIRequest{Prompt: "api_key: sk-openaisensitivevalue123456"}},
		{name: "responses", request: &dto.OpenAIResponsesRequest{Input: mustMarshal(t, "api_key: sk-responsessensitivevalue123456")}},
		{name: "responses compaction", request: &dto.OpenAIResponsesCompactionRequest{Input: mustMarshal(t, "api_key: sk-compactionsensitivevalue123456")}},
		{name: "image", request: &dto.ImageRequest{Prompt: "api_key: sk-imagesensitivevalue123456"}},
		{name: "audio", request: &dto.AudioRequest{Input: "api_key: sk-audiosensitivevalue123456"}},
		{name: "embedding", request: &dto.EmbeddingRequest{Input: "api_key: sk-embeddingsensitivevalue123456"}},
		{name: "rerank", request: &dto.RerankRequest{Query: "api_key: sk-reranksensitivevalue123456"}},
		{name: "claude", request: &dto.ClaudeRequest{Prompt: "api_key: sk-claudesensitivevalue123456"}},
		{name: "gemini chat", request: &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{{Parts: []dto.GeminiPart{{Text: "api_key: sk-geminichatsensitive123456"}}}}}},
		{name: "gemini embedding", request: &dto.GeminiEmbeddingRequest{Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: "api_key: sk-geminiembeddingsensitive123456"}}}}},
		{name: "gemini batch embedding", request: &dto.GeminiBatchEmbeddingRequest{Requests: []*dto.GeminiEmbeddingRequest{{Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: "api_key: sk-geminibatchsensitive123456"}}}}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if changed := RedactSensitiveInfoRequest(tt.request); !changed {
				t.Fatal("expected dispatcher to redact request")
			}
			body := mustMarshalString(t, tt.request)
			assertContains(t, body, "[API_KEY_REDACTED]")
			assertNotContains(t, body, "sk-")
		})
	}

	if changed := RedactSensitiveInfoRequest(&dto.BaseRequest{}); changed {
		t.Fatal("expected unsupported request type to return false")
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := common.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return data
}

func mustMarshalString(t *testing.T, v any) string {
	t.Helper()
	return string(mustMarshal(t, v))
}

func assertContains(t *testing.T, got string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(got, needle) {
			t.Fatalf("expected %q in %s", needle, got)
		}
	}
}

func assertNotContains(t *testing.T, got string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if strings.Contains(got, needle) {
			t.Fatalf("raw secret still present %q in %s", needle, got)
		}
	}
}

func stringPtr(v string) *string {
	return &v
}
