package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type DirectConsumeRequest struct {
	TokenKey         string `json:"token" binding:"required"`
	Model            string `json:"model" binding:"required"`
	PromptTokens     int    `json:"prompt_tokens" binding:"required,min=0"`
	CompletionTokens int    `json:"completion_tokens" binding:"required,min=0"`
	CacheTokens      int    `json:"cache_tokens"`
	ImageTokens      int    `json:"image_tokens"`
	IsStream         bool   `json:"is_stream"`           // 是否流式调用
	UseTime          int    `json:"use_time"`            // 总用时（毫秒）
	FirstUseTime     int    `json:"first_use_time"`      // 首字用时（毫秒）
}

type DirectConsumeResponse struct {
	Success          bool   `json:"success"`
	Message          string `json:"message,omitempty"`
	Quota            int    `json:"quota"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	ModelName        string `json:"model_name"`
	UserQuota        int    `json:"user_quota"`
	TokenQuota       int    `json:"token_quota"`
}

// DirectConsume 直接扣费接口，不调用上游API，仅根据传入的tokens数据进行扣费
func DirectConsume(c *gin.Context) {
	logger.LogInfo(c, "DirectConsume API called")

	var req DirectConsumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.LogError(c, fmt.Sprintf("Failed to bind JSON: %v", err))
		c.JSON(http.StatusBadRequest, DirectConsumeResponse{
			Success: false,
			Message: fmt.Sprintf("invalid request: %v", err),
		})
		return
	}

	logger.LogInfo(c, fmt.Sprintf("DirectConsume request: token=%s, model=%s, prompt_tokens=%d, completion_tokens=%d",
		req.TokenKey, req.Model, req.PromptTokens, req.CompletionTokens))

	// 1. 验证令牌
	token, err := model.GetTokenByKey(req.TokenKey, false)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("Token validation failed: %v", err))
		c.JSON(http.StatusUnauthorized, DirectConsumeResponse{
			Success: false,
			Message: "invalid token",
		})
		return
	}
	logger.LogInfo(c, fmt.Sprintf("Token validated: id=%d, name=%s", token.Id, token.Name))

	// 设置token name到context，用于日志记录
	c.Set("token_name", token.Name)

	if token.Status != common.TokenStatusEnabled {
		c.JSON(http.StatusForbidden, DirectConsumeResponse{
			Success: false,
			Message: "token is disabled",
		})
		return
	}

	// 检查令牌是否过期
	if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
		c.JSON(http.StatusForbidden, DirectConsumeResponse{
			Success: false,
			Message: "token has expired",
		})
		return
	}

	// 2. 获取用户信息
	user, err := model.GetUserById(token.UserId, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, DirectConsumeResponse{
			Success: false,
			Message: "failed to get user info",
		})
		return
	}

	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusForbidden, DirectConsumeResponse{
			Success: false,
			Message: "user is disabled",
		})
		return
	}

	// 设置username到context，用于日志记录
	c.Set("username", user.Username)

	// 3. 获取用户组信息和令牌组信息
	userGroup := user.Group
	tokenGroup := token.Group
	// 使用令牌的分组来计算倍率
	usingGroup := tokenGroup
	if usingGroup == "" {
		usingGroup = userGroup
	}

	// 4. 计算消耗
	modelName := req.Model
	promptTokens := req.PromptTokens
	completionTokens := req.CompletionTokens
	cacheTokens := req.CacheTokens
	imageTokens := req.ImageTokens
	totalTokens := promptTokens + completionTokens

	// 获取模型价格配置（使用令牌分组）
	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)
	completionRatio := ratio_setting.GetCompletionRatio(modelName)
	cacheRatio, _ := ratio_setting.GetCacheRatio(modelName)
	imageRatio, _ := ratio_setting.GetImageRatio(modelName)
	groupRatio := ratio_setting.GetGroupRatio(usingGroup) // 使用令牌分组
	modelPrice, usePrice := ratio_setting.GetModelPrice(modelName, false)

	var quota int
	if !usePrice {
		// 基于倍率模式计算
		baseTokens := promptTokens - cacheTokens - imageTokens
		if baseTokens < 0 {
			baseTokens = 0
		}

		// 使用 decimal 进行高精度计算
		promptQuotaDec := decimal.NewFromInt(int64(baseTokens)).
			Add(decimal.NewFromInt(int64(cacheTokens)).Mul(decimal.NewFromFloat(cacheRatio))).
			Add(decimal.NewFromInt(int64(imageTokens)).Mul(decimal.NewFromFloat(imageRatio)))

		completionQuotaDec := decimal.NewFromInt(int64(completionTokens)).
			Mul(decimal.NewFromFloat(completionRatio))

		totalQuotaDec := promptQuotaDec.Add(completionQuotaDec).
			Mul(decimal.NewFromFloat(modelRatio)).
			Mul(decimal.NewFromFloat(groupRatio))

		quota = int(totalQuotaDec.IntPart())
	} else {
		// 基于价格模式计算
		totalQuotaDec := decimal.NewFromFloat(modelPrice).
			Mul(decimal.NewFromInt(int64(totalTokens))).
			Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
			Mul(decimal.NewFromFloat(groupRatio))

		quota = int(totalQuotaDec.IntPart())
	}

	if quota < 0 {
		quota = 0
	}

	logger.LogInfo(c, fmt.Sprintf("Calculated quota: %d, modelRatio=%.2f, groupRatio=%.2f, usePrice=%v",
		quota, modelRatio, groupRatio, usePrice))

	// 5. 检查用户额度是否足够
	userQuota, err := model.GetUserQuota(user.Id, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, DirectConsumeResponse{
			Success: false,
			Message: "failed to check user quota",
		})
		return
	}

	if userQuota < quota {
		c.JSON(http.StatusPaymentRequired, DirectConsumeResponse{
			Success: false,
			Message: fmt.Sprintf("insufficient user quota, required: %s, available: %s",
				logger.FormatQuota(quota), logger.FormatQuota(userQuota)),
		})
		return
	}

	// 6. 检查令牌额度是否足够
	if !token.UnlimitedQuota && token.RemainQuota < quota {
		c.JSON(http.StatusPaymentRequired, DirectConsumeResponse{
			Success: false,
			Message: fmt.Sprintf("insufficient token quota, required: %s, available: %s",
				logger.FormatQuota(quota), logger.FormatQuota(token.RemainQuota)),
		})
		return
	}

	// 7. 构建 RelayInfo 用于记录日志
	now := time.Now()
	// 计算首字响应时间（毫秒转换）
	firstResponseTime := now
	if req.FirstUseTime > 0 {
		firstResponseTime = now.Add(time.Duration(req.FirstUseTime) * time.Millisecond)
	}

	relayInfo := &relaycommon.RelayInfo{
		UserId:            user.Id,
		TokenId:           token.Id,
		TokenKey:          req.TokenKey,
		UsingGroup:        usingGroup, // 使用令牌分组
		UserGroup:         userGroup,  // 用户分组
		TokenUnlimited:    token.UnlimitedQuota,
		IsStream:          req.IsStream,
		OriginModelName:   modelName,
		StartTime:         now,
		FirstResponseTime: firstResponseTime,
		RequestURLPath:    "/api/consume",
	}
	// 初始化 ChannelMeta 避免空指针
	relayInfo.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelId: 0, // 直接扣费没有渠道
	}

	// 8. 执行扣费
	logger.LogInfo(c, fmt.Sprintf("Starting quota consumption: quota=%d, user_id=%d, token_id=%d", quota, user.Id, token.Id))
	err = service.PostConsumeQuota(relayInfo, quota, 0, true)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("Failed to consume quota: %v", err))
		c.JSON(http.StatusInternalServerError, DirectConsumeResponse{
			Success: false,
			Message: fmt.Sprintf("failed to consume quota: %v", err),
		})
		return
	}
	logger.LogInfo(c, "Quota consumed successfully")

	// 9. 更新用户统计
	model.UpdateUserUsedQuotaAndRequestCount(user.Id, quota)
	logger.LogInfo(c, "User stats updated")

	// 10. 记录消费日志
	if common.LogConsumeEnabled {
		logger.LogInfo(c, "Recording consume log")
		otherInfo := service.GenerateTextOtherInfo(c, relayInfo, modelRatio, groupRatio,
			completionRatio, cacheTokens, cacheRatio, modelPrice, 1.0)

		content := fmt.Sprintf("模型倍率 %.2f，分组倍率 %.2f", modelRatio, groupRatio)
		if usePrice {
			content = fmt.Sprintf("模型价格 $%.6f", modelPrice)
		}

		// 计算用时（秒）
		useTimeSeconds := req.UseTime / 1000
		if useTimeSeconds == 0 && req.UseTime > 0 {
			useTimeSeconds = 1 // 至少1秒
		}

		model.RecordConsumeLog(c, user.Id, model.RecordConsumeLogParams{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			ModelName:        modelName,
			TokenName:        token.Name,
			Quota:            quota,
			Content:          content,
			UseTimeSeconds:   useTimeSeconds,
			IsStream:         req.IsStream,
			Group:            usingGroup, // 使用令牌分组
			ChannelId:        0,          // 直接扣费没有渠道
			TokenId:          token.Id,
			Other:            otherInfo,
		})
		logger.LogInfo(c, "Consume log recorded")
	} else {
		logger.LogInfo(c, "Consume log disabled")
	}

	// 11. 获取扣费后的额度
	userQuotaAfter, _ := model.GetUserQuota(user.Id, false)
	tokenQuotaAfter := token.RemainQuota - quota
	if token.UnlimitedQuota {
		tokenQuotaAfter = -1
	}

	// 12. 返回成功响应
	logger.LogInfo(c, fmt.Sprintf("DirectConsume completed successfully: quota=%d, user_quota_after=%d, token_quota_after=%d",
		quota, userQuotaAfter, tokenQuotaAfter))

	c.JSON(http.StatusOK, DirectConsumeResponse{
		Success:          true,
		Message:          "quota consumed successfully",
		Quota:            quota,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		ModelName:        modelName,
		UserQuota:        userQuotaAfter,
		TokenQuota:       tokenQuotaAfter,
	})
}
