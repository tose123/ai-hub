package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type mcpHubCheckoutRequest struct {
	Cost  string `json:"cost"`
	Info  string `json:"info"`
	Model string `json:"model"`
}

func GetMCPHubBalance(c *gin.Context) {
	if common.QuotaPerUnit <= 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "invalid quota configuration"})
		return
	}

	token, err := model.GetTokenById(common.GetContextKeyInt(c, constant.ContextKeyTokenId))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query token balance"})
		return
	}
	userQuota, err := model.GetUserQuota(common.GetContextKeyInt(c, constant.ContextKeyUserId), true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query user balance"})
		return
	}

	balance := userQuota
	if !token.UnlimitedQuota && token.RemainQuota < balance {
		balance = token.RemainQuota
	}
	amount := decimal.NewFromInt(int64(balance)).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		String()
	c.JSON(http.StatusOK, gin.H{"balance": amount})
}

func CheckoutMCPHub(c *gin.Context) {
	var req mcpHubCheckoutRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}

	costText := strings.TrimSpace(req.Cost)
	modelName := strings.TrimSpace(req.Model)
	if costText == "" || strings.TrimSpace(req.Info) == "" || modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "cost, info and model are required"})
		return
	}
	if common.QuotaPerUnit <= 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "invalid quota configuration"})
		return
	}

	cost, err := decimal.NewFromString(costText)
	if err != nil || !cost.IsPositive() {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "cost must be a positive USD amount"})
		return
	}
	quota, clamp := common.QuotaFromDecimalChecked(
		cost.Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
	if clamp != nil || quota <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "cost is outside the supported quota range"})
		return
	}

	tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if err := model.DecreaseTokenQuota(tokenId, common.GetContextKeyString(c, constant.ContextKeyTokenKey), quota); err != nil {
		common.SysLog("MCPHub token checkout failed: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to checkout"})
		return
	}
	if err := model.DecreaseUserQuota(userId, quota, false); err != nil {
		common.SysLog("MCPHub user checkout failed: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to checkout"})
		return
	}
	model.UpdateUserUsedQuotaAndRequestCount(userId, quota)

	model.RecordConsumeLog(c, userId, model.RecordConsumeLogParams{
		ModelName: modelName,
		TokenName: c.GetString("token_name"),
		Quota:     quota,
		Content:   req.Info,
		TokenId:   tokenId,
		Group:     common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
	})
	c.JSON(http.StatusOK, gin.H{"success": true})
}
