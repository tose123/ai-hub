package helper

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func GetRelayResponseHeaderTimeout(modelName string, path ...string) time.Duration {
	timeout := common.RelayResponseHeaderTimeout

	// image
	if len(path) > 0 && strings.Contains(path[0], "/images/") {
		timeout = common.RelayImageResponseHeaderTimeout
	}
	if strings.Contains(modelName, "image") ||
		strings.Contains(modelName, "gpt-image") ||
		strings.Contains(modelName, "gpt-images") ||
		strings.Contains(modelName, "dall-e") {
		timeout = common.RelayImageResponseHeaderTimeout
	}

	// TODO: video model name

	return time.Duration(timeout) * time.Second
}
