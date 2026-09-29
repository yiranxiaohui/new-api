package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// GetClientDownloads returns the latest client release for the downloads page.
func GetClientDownloads(c *gin.Context) {
	release, err := service.GetPierRelease(c.Request.Context())
	if err != nil {
		logger.LogError(c.Request.Context(), "client release lookup failed: "+err.Error())
		common.ApiErrorI18n(c, i18n.MsgClientDownloadUnavailable)
		return
	}
	common.ApiSuccess(c, release.WithMirror(operation_setting.GetGeneralSetting().ClientDownloadMirror))
}
