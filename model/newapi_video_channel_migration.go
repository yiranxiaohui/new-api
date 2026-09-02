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
//
// hadLegacySchema reports whether the channels table existed before this
// boot's AutoMigrate. A fresh database has nothing to renumber, so only the
// marker is written; without it the first real Task Plugin channel created
// after install would be renumbered on the next restart.
func migrateNewAPIVideoChannelType(db *gorm.DB, hadLegacySchema bool) error {
	if db == nil {
		return errors.New("database is not initialized")
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
		if !hadLegacySchema {
			if err := tx.Create(&Option{Key: newAPIVideoChannelMigrationKey, Value: strconv.Itoa(constant.ChannelTypeNewAPIVideo)}).Error; err != nil {
				return fmt.Errorf("record %s: %w", newAPIVideoChannelMigrationKey, err)
			}
			return nil
		}

		channels := tx.Model(&Channel{}).Where("type = ?", legacyNewAPIVideoChannelType).Update("type", constant.ChannelTypeNewAPIVideo)
		if channels.Error != nil {
			return fmt.Errorf("renumber New API Video channels: %w", channels.Error)
		}
		tasks := tx.Model(&Task{}).
			Where("platform = ?", strconv.Itoa(legacyNewAPIVideoChannelType)).
			Update("platform", strconv.Itoa(constant.ChannelTypeNewAPIVideo))
		if tasks.Error != nil {
			return fmt.Errorf("renumber New API Video tasks: %w", tasks.Error)
		}
		if err := tx.Create(&Option{Key: newAPIVideoChannelMigrationKey, Value: strconv.Itoa(constant.ChannelTypeNewAPIVideo)}).Error; err != nil {
			return fmt.Errorf("record %s: %w", newAPIVideoChannelMigrationKey, err)
		}
		if channels.RowsAffected > 0 || tasks.RowsAffected > 0 {
			common.SysLog(fmt.Sprintf("migrated New API Video channel type %d -> %d: channels=%d tasks=%d",
				legacyNewAPIVideoChannelType, constant.ChannelTypeNewAPIVideo, channels.RowsAffected, tasks.RowsAffected))
		}
		return nil
	})
}
