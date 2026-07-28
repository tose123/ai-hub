package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitTaskKeepsInternalModelNamePrivate(t *testing.T) {
	info := &relaycommon.RelayInfo{
		UserId:              1,
		UsingGroup:          "default",
		RequestModelName:    "alias-a",
		ExternalModelName:   "model-b",
		ExternalModelMapped: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         2,
			UpstreamModelName: "model-c",
		},
	}

	task := InitTask(constant.TaskPlatformSuno, info)

	require.NotNil(t, task)
	assert.Equal(t, "model-b", task.Properties.OriginModelName)
	assert.Empty(t, task.Properties.UpstreamModelName)
	assert.Equal(t, "alias-a", task.PrivateData.RequestModelName)
	assert.True(t, task.PrivateData.ExternalModelMapped)
	assert.Equal(t, "model-c", task.PrivateData.UpstreamModelName)
}
