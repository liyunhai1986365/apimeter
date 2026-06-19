package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const userOwnedProviderGroupPrefix = "user_owned"

func BuildUserOwnedProviderGroup(userID int, channelID int) string {
	return fmt.Sprintf("%s:%d:%d", userOwnedProviderGroupPrefix, userID, channelID)
}

func ParseUserOwnedProviderGroup(group string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(group), ":")
	if len(parts) != 3 || parts[0] != userOwnedProviderGroupPrefix {
		return 0, 0, false
	}
	userID, err := strconv.Atoi(parts[1])
	if err != nil || userID <= 0 {
		return 0, 0, false
	}
	channelID, err := strconv.Atoi(parts[2])
	if err != nil || channelID <= 0 {
		return 0, 0, false
	}
	return userID, channelID, true
}

func IsUserOwnedProviderGroup(group string) bool {
	_, _, ok := ParseUserOwnedProviderGroup(group)
	return ok
}

func UserOwnsProviderGroup(userID int, group string) (bool, error) {
	groupUserID, channelID, ok := ParseUserOwnedProviderGroup(group)
	if !ok || groupUserID != userID {
		return false, nil
	}

	var count int64
	err := DB.Model(&Channel{}).
		Where("id = ? AND scope = ? AND owner_user_id = ? AND "+commonGroupCol+" = ?", channelID, ChannelScopeUserOwned, userID, group).
		Count(&count).Error
	return count > 0, err
}

func GetUserOwnedProviderChannelForGroup(userID int, group string, modelName string) (*Channel, error) {
	groupUserID, channelID, ok := ParseUserOwnedProviderGroup(group)
	if !ok || groupUserID != userID {
		return nil, errors.New("无权访问自有供应商分组")
	}

	channel := &Channel{}
	err := DB.Where("id = ? AND scope = ? AND owner_user_id = ? AND "+commonGroupCol+" = ? AND status = ?",
		channelID, ChannelScopeUserOwned, userID, group, common.ChannelStatusEnabled).
		First(channel).Error
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(modelName) == "" {
		return channel, nil
	}
	for _, item := range channel.GetModels() {
		if item == modelName {
			return channel, nil
		}
	}
	return nil, fmt.Errorf("自有供应商分组不支持模型 %s", modelName)
}

func ListUserOwnedProviderChannels(userID int) ([]Channel, error) {
	channels := make([]Channel, 0)
	err := DB.Omit("key").
		Where("scope = ? AND owner_user_id = ?", ChannelScopeUserOwned, userID).
		Order("id DESC").
		Find(&channels).Error
	return channels, err
}
