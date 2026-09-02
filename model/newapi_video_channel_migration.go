package model

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// legacyNewAPIVideoChannelType is the channel type this fork used for
// "New API Video" before upstream reserved 61 for "Task Plugin" channels.
const legacyNewAPIVideoChannelType = 61

// newAPIVideoChannelMigrationKey records that the one-time renumbering ran, so
// genuine Task Plugin channels created after the switch are never touched.
const newAPIVideoChannelMigrationKey = "migration.newapi_video_channel_type"

// migrateNewAPIVideoChannelType renumbers channels and tasks that were stored
// under the fork's legacy type 61 to ChannelTypeNewAPIVideo. It runs exactly
// once per database; afterwards type 61 belongs to upstream's Task Plugin.
func migrateNewAPIVideoChannelType(db *gorm.DB) error {
	if db == nil {
		return errors.New("database is not initialized")
	}
	if !db.Migrator().HasTable(&Option{}) || !db.Migrator().HasTable(&Channel{}) {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var marker Option
		err := tx.Where(&Option{Key: newAPIVideoChannelMigrationKey}).First(&marker).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read %s: %w", newAPIVideoChannelMigrationKey, err)
		}

		channels := tx.Model(&Channel{}).Where("type = ?", legacyNewAPIVideoChannelType).Update("type", constant.ChannelTypeNewAPIVideo)
		if channels.Error != nil {
			return fmt.Errorf("renumber New API Video channels: %w", channels.Error)
		}
		var tasks *gorm.DB
		if tx.Migrator().HasTable(&Task{}) {
			tasks = tx.Model(&Task{}).
				Where("platform = ?", strconv.Itoa(legacyNewAPIVideoChannelType)).
				Update("platform", strconv.Itoa(constant.ChannelTypeNewAPIVideo))
			if tasks.Error != nil {
				return fmt.Errorf("renumber New API Video tasks: %w", tasks.Error)
			}
		}
		if err := tx.Create(&Option{Key: newAPIVideoChannelMigrationKey, Value: strconv.Itoa(constant.ChannelTypeNewAPIVideo)}).Error; err != nil {
			return fmt.Errorf("record %s: %w", newAPIVideoChannelMigrationKey, err)
		}
		if channels.RowsAffected > 0 || (tasks != nil && tasks.RowsAffected > 0) {
			taskRows := int64(0)
			if tasks != nil {
				taskRows = tasks.RowsAffected
			}
			common.SysLog(fmt.Sprintf("migrated New API Video channel type %d -> %d: channels=%d tasks=%d",
				legacyNewAPIVideoChannelType, constant.ChannelTypeNewAPIVideo, channels.RowsAffected, taskRows))
		}
		return nil
	})
}
