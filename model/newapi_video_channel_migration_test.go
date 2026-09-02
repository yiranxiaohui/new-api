package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newNewAPIVideoMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}, &Channel{}, &Task{}))
	return db
}

func TestMigrateNewAPIVideoChannelTypeRenumbersLegacyRows(t *testing.T) {
	db := newNewAPIVideoMigrationDB(t)
	require.NoError(t, db.Create(&Channel{Id: 1, Type: legacyNewAPIVideoChannelType, Name: "video-relay", Key: "k"}).Error)
	require.NoError(t, db.Create(&Channel{Id: 2, Type: constant.ChannelTypeOpenAI, Name: "openai", Key: "k"}).Error)
	require.NoError(t, db.Create(&Task{TaskID: "task_a", Platform: constant.TaskPlatform("61"), ChannelId: 1}).Error)
	require.NoError(t, db.Create(&Task{TaskID: "task_b", Platform: constant.TaskPlatform("55"), ChannelId: 2}).Error)

	require.NoError(t, migrateNewAPIVideoChannelType(db, true))

	var video, openai Channel
	require.NoError(t, db.First(&video, 1).Error)
	require.NoError(t, db.First(&openai, 2).Error)
	assert.Equal(t, constant.ChannelTypeNewAPIVideo, video.Type)
	assert.Equal(t, constant.ChannelTypeOpenAI, openai.Type)

	var taskA, taskB Task
	require.NoError(t, db.Where("task_id = ?", "task_a").First(&taskA).Error)
	require.NoError(t, db.Where("task_id = ?", "task_b").First(&taskB).Error)
	assert.Equal(t, constant.TaskPlatform("62"), taskA.Platform)
	assert.Equal(t, constant.TaskPlatform("55"), taskB.Platform)

	var marker Option
	require.NoError(t, db.Where(&Option{Key: newAPIVideoChannelMigrationKey}).First(&marker).Error)
	assert.Equal(t, "62", marker.Value)
}

func TestMigrateNewAPIVideoChannelTypeLeavesTaskPluginChannelsAfterFirstRun(t *testing.T) {
	db := newNewAPIVideoMigrationDB(t)
	require.NoError(t, migrateNewAPIVideoChannelType(db, true))

	setting := `{"task_plugin_key":"kling"}`
	require.NoError(t, db.Create(&Channel{Id: 3, Type: constant.ChannelTypeTaskPlugin, Name: "plugin", Key: "k", Setting: &setting}).Error)
	require.NoError(t, db.Create(&Task{TaskID: "task_c", Platform: constant.TaskPlatform("61"), ChannelId: 3}).Error)

	require.NoError(t, migrateNewAPIVideoChannelType(db, true))

	var plugin Channel
	require.NoError(t, db.First(&plugin, 3).Error)
	assert.Equal(t, constant.ChannelTypeTaskPlugin, plugin.Type)
	var taskC Task
	require.NoError(t, db.Where("task_id = ?", "task_c").First(&taskC).Error)
	assert.Equal(t, constant.TaskPlatform("61"), taskC.Platform)
}

func TestMigrateNewAPIVideoChannelTypeOnFreshInstallOnlyRecordsMarker(t *testing.T) {
	db := newNewAPIVideoMigrationDB(t)

	// Boot 1: the tables were just created by AutoMigrate.
	require.NoError(t, migrateNewAPIVideoChannelType(db, false))
	var marker Option
	require.NoError(t, db.Where(&Option{Key: newAPIVideoChannelMigrationKey}).First(&marker).Error)

	// Operator binds a real Task Plugin channel before the next restart.
	setting := `{"task_plugin_key":"sora"}`
	require.NoError(t, db.Create(&Channel{Id: 7, Type: constant.ChannelTypeTaskPlugin, Name: "plugin", Key: "k", Setting: &setting}).Error)
	require.NoError(t, db.Create(&Task{TaskID: "task_p", Platform: constant.TaskPlatform("61"), ChannelId: 7}).Error)

	// Boot 2: tables now exist, but the marker must stop the renumbering.
	require.NoError(t, migrateNewAPIVideoChannelType(db, true))
	var plugin Channel
	require.NoError(t, db.First(&plugin, 7).Error)
	assert.Equal(t, constant.ChannelTypeTaskPlugin, plugin.Type)
	var task Task
	require.NoError(t, db.Where("task_id = ?", "task_p").First(&task).Error)
	assert.Equal(t, constant.TaskPlatform("61"), task.Platform)
}
