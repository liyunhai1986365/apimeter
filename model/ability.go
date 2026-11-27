package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"

	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Ability struct {
	Group     string  `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model     string  `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool    `json:"enabled"`
	Priority  *int64  `json:"priority" gorm:"bigint;default:0;index"`
	Weight    uint    `json:"weight" gorm:"default:0;index"`
	Tag       *string `json:"tag" gorm:"index"`
}

type AbilityWithChannel struct {
	Ability
	ChannelType int `json:"channel_type"`
}

func GetAllEnableAbilityWithChannels() ([]AbilityWithChannel, error) {
	var abilities []AbilityWithChannel
	err := DB.Table("abilities").
		Select("abilities.*, channels.type as channel_type").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Scan(&abilities).Error
	return abilities, err
}

func GetGroupEnabledModels(group string) []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where(commonGroupCol+" = ? and enabled = ?", group, true).Distinct("model").Pluck("model", &models)
	return models
}

// GetEnabledModelsInGroupOrder returns a shared model at its first enabled group.
func GetEnabledModelsInGroupOrder(groups []string) []string {
	var names []string
	// Each group binds once in IN and once in CASE. Leave room for the
	// enabled flag on SQLite builds with a 999-parameter limit.
	for _, batch := range lo.Chunk(lo.Uniq(groups), 400) {
		if len(batch) == 1 {
			names = append(names, GetGroupEnabledModels(batch[0])...)
			continue
		}
		var order strings.Builder
		order.WriteString("CASE " + commonGroupCol)
		args := make([]any, 0, len(batch))
		for index, group := range batch {
			fmt.Fprintf(&order, " WHEN ? THEN %d", index)
			args = append(args, group)
		}
		order.WriteString(" END, model")
		var batchNames []string
		DB.Model(&Ability{}).Where(commonGroupCol+" IN ? AND enabled = ?", batch, true).
			Group(commonGroupCol+", model").
			Clauses(clause.OrderBy{Expression: clause.Expr{SQL: order.String(), Vars: args}}).
			Pluck("model", &batchNames)
		names = append(names, batchNames...)
	}
	return lo.Uniq(names)
}

func GetEnabledModels() []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where("enabled = ?", true).Distinct("model").Pluck("model", &models)
	return models
}

func GetAllEnableAbilities() []Ability {
	var abilities []Ability
	DB.Find(&abilities, "enabled = ?", true)
	return abilities
}

// Ability rows can outlive a group assignment. Channel ownership is the
// authority for shared routing, including priority and affinity lookups.
func sharedRoutingAbilities() *gorm.DB {
	return DB.Model(&Ability{}).Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("(channels.scope IS NULL OR channels.scope <> ?) AND abilities.enabled = ?", ChannelScopeUserOwned, true)
}

func GetChannel(group string, model string, retry int) (*Channel, error) {
	return GetChannelWithFilter(group, model, retry, nil)
}

func GetChannelWithFilter(group string, model string, retry int, filter ChannelFilter) (*Channel, error) {
	var abilities []Ability

	var err error = nil
	channelQuery, err := getFilteredChannelQuery(group, model, filter)
	if err != nil {
		return nil, err
	}
	if retry != 0 {
		priority, err := getPriorityFromQuery(channelQuery, retry)
		if err != nil {
			return nil, err
		}
		channelQuery = channelQuery.Where("abilities.priority = ?", priority)
	} else {
		channelQuery = channelQuery.Where("abilities.priority = (?)", channelQuery.Session(&gorm.Session{}).Select("MAX(abilities.priority)"))
	}
	err = channelQuery.Order("abilities.weight DESC").Find(&abilities).Error
	if err != nil {
		return nil, err
	}
	channel := Channel{}
	if len(abilities) > 0 {
		// Randomly choose one
		weightSum := uint(0)
		for _, ability_ := range abilities {
			weightSum += ability_.Weight + 10
		}
		// Randomly choose one
		weight := common.GetRandomInt(int(weightSum))
		for _, ability_ := range abilities {
			weight -= int(ability_.Weight) + 10
			//log.Printf("weight: %d, ability weight: %d", weight, *ability_.Weight)
			if weight <= 0 {
				channel.Id = ability_.ChannelId
				break
			}
		}
	} else {
		return nil, nil
	}
	err = DB.First(&channel, "id = ?", channel.Id).Error
	return RequirePlatformChannel(&channel, err)
}

func getFilteredChannelQuery(group string, model string, filter ChannelFilter) (*gorm.DB, error) {
	channelQuery := sharedRoutingAbilities().Where("abilities."+commonGroupCol+" = ? AND abilities.model = ?", group, model)
	if filter == nil {
		return channelQuery, nil
	}

	var abilities []Ability
	if err := channelQuery.Session(&gorm.Session{}).Find(&abilities).Error; err != nil {
		return nil, err
	}
	channelIDs := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		channel := Channel{}
		if err := DB.First(&channel, "id = ?", ability.ChannelId).Error; err != nil {
			return nil, err
		}
		if filter(&channel) {
			channelIDs = append(channelIDs, ability.ChannelId)
		}
	}
	if len(channelIDs) == 0 {
		return nil, ErrNoChannelMatchedFilter
	}
	return channelQuery.Where("abilities.channel_id IN ?", channelIDs), nil
}

func getPriorityFromQuery(channelQuery *gorm.DB, retry int) (int, error) {
	var priorities []int
	err := channelQuery.Session(&gorm.Session{}).
		Distinct("abilities.priority").
		Order("abilities.priority DESC").
		Pluck("abilities.priority", &priorities).Error
	if err != nil {
		return 0, err
	}
	if len(priorities) == 0 {
		return 0, errors.New("数据库一致性被破坏")
	}
	if retry >= len(priorities) {
		return priorities[len(priorities)-1], nil
	}
	return priorities[retry], nil
}

func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	models_ := channel.GetModels()
	groups_ := channel.GetGroups()
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}
	if len(abilities) == 0 {
		return nil
	}
	// choose DB or provided tx
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		err := useDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	isNewTx := false
	// 如果没有传入事务，创建新的事务
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}

	// First delete all abilities of this channel
	err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}

	// Then add new abilities
	models_ := channel.GetModels()
	groups_ := channel.GetGroups()
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}

	if len(abilities) > 0 {
		for _, chunk := range lo.Chunk(abilities, 50) {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
			if err != nil {
				if isNewTx {
					tx.Rollback()
				}
				return err
			}
		}
	}

	// 如果是新创建的事务，需要提交
	if isNewTx {
		return tx.Commit().Error
	}

	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityStatusByTag(tag string, status bool) error {
	return DB.Model(&Ability{}).Where("tag = ?", tag).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityByTag(tag string, newTag *string, priority *int64, weight *uint) error {
	ability := Ability{}
	if newTag != nil {
		ability.Tag = newTag
	}
	if priority != nil {
		ability.Priority = priority
	}
	if weight != nil {
		ability.Weight = *weight
	}
	return DB.Model(&Ability{}).Where("tag = ?", tag).Updates(ability).Error
}

var fixLock = sync.Mutex{}

func FixAbility() (int, int, error) {
	lock := fixLock.TryLock()
	if !lock {
		return 0, 0, errors.New("已经有一个修复任务在运行中，请稍后再试")
	}
	defer fixLock.Unlock()

	// truncate abilities table
	if common.UsingSQLite {
		err := DB.Exec("DELETE FROM abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	} else {
		err := DB.Exec("TRUNCATE TABLE abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Truncate abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	}
	var channels []*Channel
	// Find all channels
	err := DB.Model(&Channel{}).Find(&channels).Error
	if err != nil {
		return 0, 0, err
	}
	if len(channels) == 0 {
		return 0, 0, nil
	}
	successCount := 0
	failCount := 0
	for _, chunk := range lo.Chunk(channels, 50) {
		ids := lo.Map(chunk, func(c *Channel, _ int) int { return c.Id })
		// Delete all abilities of this channel
		err = DB.Where("channel_id IN ?", ids).Delete(&Ability{}).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			failCount += len(chunk)
			continue
		}
		// Then add new abilities
		for _, channel := range chunk {
			err = channel.AddAbilities(nil)
			if err != nil {
				common.SysLog(fmt.Sprintf("Add abilities for channel %d failed: %s", channel.Id, err.Error()))
				failCount++
			} else {
				successCount++
			}
		}
	}
	InitChannelCache()
	return successCount, failCount, nil
}
