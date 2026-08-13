package helper

import (
	"errors"
	"fmt"
	"strings"

	corecommon "github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

func ModelMappedHelper(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}
	originModelName := info.OriginModelName
	baseModelName := model_setting.BaseModelForMatching(originModelName)
	modelSuffix := ""
	if strings.HasPrefix(originModelName, baseModelName) {
		modelSuffix = strings.TrimPrefix(originModelName, baseModelName)
	}
	mappingModelName := baseModelName

	// map model name
	modelMapping := c.GetString("model_mapping")
	if modelMapping != "" && modelMapping != "{}" {
		modelMap := make(map[string]string)
		err := corecommon.UnmarshalJsonStr(modelMapping, &modelMap)
		if err != nil {
			return fmt.Errorf("unmarshal_model_mapping_failed")
		}

		// 支持链式模型重定向，最终使用链尾的模型
		currentModel := mappingModelName
		visitedModels := map[string]bool{
			currentModel: true,
		}
		for {
			if mappedModel, exists := modelMap[currentModel]; exists && mappedModel != "" {
				// 模型重定向循环检测，避免无限循环
				if visitedModels[mappedModel] {
					if mappedModel == currentModel {
						if currentModel == info.OriginModelName {
							info.IsModelMapped = false
							return nil
						} else {
							info.IsModelMapped = true
							break
						}
					}
					return errors.New("model_mapping_contains_cycle")
				}
				visitedModels[mappedModel] = true
				currentModel = mappedModel
				info.IsModelMapped = true
			} else {
				break
			}
		}
		if info.IsModelMapped {
			if model_setting.BaseModelForMatching(currentModel) == currentModel {
				currentModel += modelSuffix
			}
			info.UpstreamModelName = currentModel
		}
	}
	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	if info.UpstreamModelName != "" {
		corecommon.SetContextKey(c, appconstant.ContextKeyUpstreamModel, info.UpstreamModelName)
	}
	return nil
}
