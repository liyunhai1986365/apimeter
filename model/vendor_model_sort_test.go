package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSortOrderTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originDB := DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Model{}, &Vendor{}))
	DB = db

	t.Cleanup(func() { DB = originDB })
	return db
}

func TestGetAllVendorsOrdersBySortOrderThenID(t *testing.T) {
	setupSortOrderTestDB(t)

	late := Vendor{Name: "Late", SortOrder: 30, Status: 1}
	first := Vendor{Name: "First", SortOrder: 10, Status: 1}
	second := Vendor{Name: "Second", SortOrder: 10, Status: 1}
	require.NoError(t, late.Insert())
	require.NoError(t, first.Insert())
	require.NoError(t, second.Insert())

	vendors, err := GetAllVendors(0, 10)
	require.NoError(t, err)
	require.Len(t, vendors, 3)
	require.Equal(t, []string{"First", "Second", "Late"}, []string{
		vendors[0].Name,
		vendors[1].Name,
		vendors[2].Name,
	})
}

func TestVendorInsertDefaultsSortOrderTo100(t *testing.T) {
	setupSortOrderTestDB(t)

	vendor := Vendor{Name: "Defaulted", Status: 1}
	require.NoError(t, vendor.Insert())

	var persisted Vendor
	require.NoError(t, DB.First(&persisted, vendor.Id).Error)
	require.Equal(t, 100, persisted.SortOrder)
}

func TestUpdateVendorSortOrdersPersistsBatchOrder(t *testing.T) {
	setupSortOrderTestDB(t)

	first := Vendor{Name: "First", SortOrder: 10, Status: 1}
	second := Vendor{Name: "Second", SortOrder: 20, Status: 1}
	require.NoError(t, first.Insert())
	require.NoError(t, second.Insert())

	err := UpdateVendorSortOrders([]SortOrderUpdate{
		{ID: first.Id, SortOrder: 200},
		{ID: second.Id, SortOrder: 100},
	})
	require.NoError(t, err)

	vendors, err := GetAllVendors(0, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"Second", "First"}, []string{
		vendors[0].Name,
		vendors[1].Name,
	})
}

func TestGetAllModelsOrdersBySortOrderThenID(t *testing.T) {
	setupSortOrderTestDB(t)

	late := Model{ModelName: "late", SortOrder: 30, Status: 1, SyncOfficial: 1}
	first := Model{ModelName: "first", SortOrder: 10, Status: 1, SyncOfficial: 1}
	second := Model{ModelName: "second", SortOrder: 10, Status: 1, SyncOfficial: 1}
	require.NoError(t, late.Insert())
	require.NoError(t, first.Insert())
	require.NoError(t, second.Insert())

	models, err := GetAllModels(0, 10)
	require.NoError(t, err)
	require.Len(t, models, 3)
	require.Equal(t, []string{"first", "second", "late"}, []string{
		models[0].ModelName,
		models[1].ModelName,
		models[2].ModelName,
	})
}

func TestModelInsertDefaultsSortOrderTo100(t *testing.T) {
	setupSortOrderTestDB(t)

	model := Model{ModelName: "defaulted", Status: 1, SyncOfficial: 1}
	require.NoError(t, model.Insert())

	var persisted Model
	require.NoError(t, DB.First(&persisted, model.Id).Error)
	require.Equal(t, 100, persisted.SortOrder)
}

func TestUpdateModelSortOrdersPersistsBatchOrder(t *testing.T) {
	setupSortOrderTestDB(t)

	first := Model{ModelName: "first", SortOrder: 10, Status: 1, SyncOfficial: 1}
	second := Model{ModelName: "second", SortOrder: 20, Status: 1, SyncOfficial: 1}
	require.NoError(t, first.Insert())
	require.NoError(t, second.Insert())

	err := UpdateModelSortOrders([]SortOrderUpdate{
		{ID: first.Id, SortOrder: 200},
		{ID: second.Id, SortOrder: 100},
	})
	require.NoError(t, err)

	models, err := GetAllModels(0, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"second", "first"}, []string{
		models[0].ModelName,
		models[1].ModelName,
	})
}
