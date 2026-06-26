package perfmetrics

import (
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRecordRelaySampleSkipsNotImplementedError(t *testing.T) {
	hotBuckets = sync.Map{}

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-4o",
		UsingGroup:      "default",
		StartTime:       time.Now().Add(-time.Second),
		LastError: types.NewErrorWithStatusCode(
			errors.New("status_code=500, not implemented"),
			types.ErrorCodeDoRequestFailed,
			http.StatusInternalServerError,
		),
	}

	RecordRelaySample(info, false, 0)

	empty := true
	hotBuckets.Range(func(_, _ any) bool {
		empty = false
		return false
	})
	require.True(t, empty)
}
