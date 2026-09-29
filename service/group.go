package service

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	if userGroup != "" {
		selfRemoved := false
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if after, ok := strings.CutPrefix(specialGroup, "-:"); ok {
					// 移除分组
					groupToRemove := after
					delete(groupsCopy, groupToRemove)
					if groupToRemove == userGroup {
						selfRemoved = true
					}
				} else if after, ok := strings.CutPrefix(specialGroup, "+:"); ok {
					// 添加分组
					groupToAdd := after
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup。
		// 但当用户组通过 "-:自身" 显式移除自己时,尊重该规则,不再自动加回。
		if _, ok := groupsCopy[userGroup]; !ok && !selfRemoved {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

func IsUserSelectableGroup(userGroup, groupName string) bool {
	if groupName == "" || groupName == "auto" {
		return false
	}
	return GroupInUserUsableGroups(userGroup, groupName) && ratio_setting.ContainsGroupRatio(groupName)
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	autoGroups := make([]string, 0)
	seen := make(map[string]struct{})
	for _, group := range setting.GetAutoGroups() {
		if !IsUserSelectableGroup(userGroup, group) {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		autoGroups = append(autoGroups, group)
	}
	return autoGroups
}

// FilterUserTokenAutoGroups applies current permissions before the current
// per-token limit. It intentionally does not fall back to the global Auto list.
func FilterUserTokenAutoGroups(userGroup string, groups []string) []string {
	maxCount := setting.GetMaxTokenAutoGroups()
	filtered := make([]string, 0, min(len(groups), maxCount))
	seen := make(map[string]struct{})
	for _, group := range groups {
		if !IsUserSelectableGroup(userGroup, group) {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		filtered = append(filtered, group)
		if len(filtered) == maxCount {
			break
		}
	}
	return filtered
}

// GetRequestAutoGroups resolves the ordered Auto groups for the current token.
// The absence of the context value means that the token inherits the complete
// global Auto list; a present (even empty) value is an explicit token snapshot.
func GetRequestAutoGroups(c *gin.Context, userGroup string) []string {
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenAutoGroups)
	if !ok {
		return GetUserAutoGroup(userGroup)
	}
	groups, ok := value.([]string)
	if !ok {
		return []string{}
	}
	return FilterUserTokenAutoGroups(userGroup, groups)
}

// GetGroupsEnabledModels 按 groups 顺序获取各分组启用的模型并去重
func GetGroupsEnabledModels(groups []string) []string {
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for _, group := range groups {
		for _, modelName := range model.GetGroupEnabledModels(group) {
			if _, ok := seen[modelName]; !ok {
				seen[modelName] = struct{}{}
				models = append(models, modelName)
			}
		}
	}
	return models
}

// ResolveGroupRatio 计算用户使用 usingGroup 分组时实际计费的分组倍率，计费与展示共用：
//  1. 管理员为该用户单独设置的分组倍率（userGroupRatios）是最终值，不再叠加用户倍率；
//  2. 否则取分组间特殊倍率（用户分组 → 使用分组），没有时取分组倍率，再乘以用户倍率。
//
// userRatio 为归一化后的用户倍率（未设置时为 1）。
func ResolveGroupRatio(userGroup, usingGroup string, userRatio float64, userGroupRatios map[string]float64) hosttypes.GroupRatioInfo {
	if ratio, ok := userGroupRatios[usingGroup]; ok {
		return hosttypes.GroupRatioInfo{
			GroupRatio:        ratio,
			GroupSpecialRatio: ratio,
			HasSpecialRatio:   true,
		}
	}

	info := hosttypes.GroupRatioInfo{GroupSpecialRatio: -1}
	if ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, usingGroup); ok {
		info.GroupRatio = ratio
		info.GroupSpecialRatio = ratio
		info.HasSpecialRatio = true
	} else {
		info.GroupRatio = ratio_setting.GetGroupRatio(usingGroup)
	}

	if userRatio > 0 && userRatio != 1 {
		info.GroupRatio *= userRatio
		if info.HasSpecialRatio {
			info.GroupSpecialRatio = info.GroupRatio
		}
	}
	return info
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}

// GetUserEffectiveGroupRatio 返回用户使用某个分组时实际计费的分组倍率，
// 规则与 ResolveGroupRatio 一致，供密钥管理、分组选择和模型广场展示。
func GetUserEffectiveGroupRatio(userGroup, group string, userRatio float64, userGroupRatios map[string]float64) float64 {
	ratio := ResolveGroupRatio(userGroup, group, userRatio, userGroupRatios).GroupRatio
	// 浮点乘积会产生 1.2000000000000002 之类的尾数，展示前按 6 位小数收敛。
	return math.Round(ratio*1e6) / 1e6
}
