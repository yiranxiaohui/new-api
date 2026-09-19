package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]any)
	userGroup := ""
	userRatio := 1.0
	userId := c.GetInt("id")
	// 已登录用户读缓存，一次取到分组与专属倍率；匿名访问回退到分组查询。
	if user, err := model.GetUserCache(userId); err == nil {
		userGroup = user.Group
		userRatio = user.GetRatio()
	} else {
		userGroup, _ = model.GetUserGroup(userId, false)
	}
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			baseRatio := service.GetUserGroupRatio(userGroup, groupName)
			usableGroups[groupName] = map[string]any{
				// ratio 是用户实际生效的倍率（已乘上用户专属倍率），
				// base_ratio 保留分组自身的倍率，供前端展示换算过程。
				"ratio":      service.ApplyUserRatio(baseRatio, userRatio),
				"base_ratio": baseRatio,
				"desc":       desc,
			}
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]any{
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "",
		"data":       usableGroups,
		"user_ratio": userRatio,
	})
}
