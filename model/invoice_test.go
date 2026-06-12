package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

// migrateInvoiceTables 幂等迁移本功能所需表（TestMain 的列表不含发票表）
func migrateInvoiceTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Invoice{}, &InvoiceOrder{}, &InvoiceFile{}, &TopUp{}, &SubscriptionOrder{}))
}

func clearInvoiceTables(t *testing.T) {
	t.Helper()
	migrateInvoiceTables(t)
	require.NoError(t, DB.Exec("DELETE FROM invoice_files").Error)
	require.NoError(t, DB.Exec("DELETE FROM invoice_orders").Error)
	require.NoError(t, DB.Exec("DELETE FROM invoices").Error)
	require.NoError(t, DB.Exec("DELETE FROM top_ups").Error)
	require.NoError(t, DB.Exec("DELETE FROM subscription_orders").Error)
}

func TestInvoiceOrderUniqueIndex(t *testing.T) {
	clearInvoiceTables(t)
	inv := &Invoice{UserId: 1, InvoiceNo: "INV-T1", TitleType: InvoiceTitleTypePersonal,
		TitleName: "张三", Email: "a@b.c", Money: 10, Status: InvoiceStatusPending, CreateTime: 1}
	require.NoError(t, DB.Create(inv).Error)
	o1 := &InvoiceOrder{InvoiceId: inv.Id, OrderType: InvoiceOrderTypeTopup, OrderId: 100, TradeNo: "T100", Money: 10}
	require.NoError(t, DB.Create(o1).Error)
	// 同一订单再次挂接必须违反唯一索引
	o2 := &InvoiceOrder{InvoiceId: inv.Id + 999, OrderType: InvoiceOrderTypeTopup, OrderId: 100, TradeNo: "T100", Money: 10}
	require.Error(t, DB.Create(o2).Error)
}

func seedSuccessTopup(t *testing.T, id, userId int, money float64) {
	t.Helper()
	require.NoError(t, DB.Create(&TopUp{Id: id, UserId: userId, Money: money,
		TradeNo: fmt.Sprintf("TP%d", id), Status: common.TopUpStatusSuccess, CreateTime: 1000}).Error)
}

func TestCreateInvoiceWithOrders(t *testing.T) {
	clearInvoiceTables(t)
	seedSuccessTopup(t, 1, 7, 10.5)
	seedSuccessTopup(t, 2, 7, 4.5)

	inv := &Invoice{UserId: 7, TitleType: InvoiceTitleTypeCompany, TitleName: "X公司",
		TaxNo: "91xxx", Email: "x@y.z"}
	orders := []*InvoiceOrder{
		{OrderType: InvoiceOrderTypeTopup, OrderId: 1, TradeNo: "TP1", Money: 10.5},
		{OrderType: InvoiceOrderTypeTopup, OrderId: 2, TradeNo: "TP2", Money: 4.5},
	}
	require.NoError(t, CreateInvoiceWithOrders(inv, orders))
	require.Equal(t, 15.0, inv.Money)         // 服务端累加
	require.Equal(t, InvoiceStatusPending, inv.Status)
	require.NotEmpty(t, inv.InvoiceNo)

	// 同一订单重复申请必须失败
	inv2 := &Invoice{UserId: 7, TitleType: 1, TitleName: "张三", Email: "x@y.z"}
	err := CreateInvoiceWithOrders(inv2, []*InvoiceOrder{
		{OrderType: InvoiceOrderTypeTopup, OrderId: 1, TradeNo: "TP1", Money: 10.5},
	})
	require.ErrorIs(t, err, ErrInvoiceOrderOccupied)
	// 失败时不残留主记录
	var cnt int64
	require.NoError(t, DB.Model(&Invoice{}).Count(&cnt).Error)
	require.EqualValues(t, 1, cnt)
}

func TestCreateInvoiceWithOrdersEmptyGuard(t *testing.T) {
	// 空订单列表
	require.ErrorIs(t, CreateInvoiceWithOrders(&Invoice{UserId: 7, TitleName: "x", Email: "a@b.c"}, nil), ErrInvoiceNoOrders)
}

func TestGetInvoiceableOrders(t *testing.T) {
	clearInvoiceTables(t)
	seedSuccessTopup(t, 1, 7, 10)
	seedSuccessTopup(t, 2, 7, 20)
	seedSuccessTopup(t, 3, 8, 30) // 其他用户
	require.NoError(t, DB.Create(&TopUp{Id: 4, UserId: 7, Money: 40, TradeNo: "TP4",
		Status: common.TopUpStatusPending, CreateTime: 1000}).Error) // 未支付
	require.NoError(t, DB.Create(&SubscriptionOrder{Id: 9, UserId: 7, Money: 9.9,
		TradeNo: "SUB9", Status: common.TopUpStatusSuccess, CreateTime: 1000}).Error)

	// 占用订单 1
	inv := &Invoice{UserId: 7, TitleType: 1, TitleName: "张三", Email: "a@b.c"}
	require.NoError(t, CreateInvoiceWithOrders(inv, []*InvoiceOrder{
		{OrderType: InvoiceOrderTypeTopup, OrderId: 1, TradeNo: "TP1", Money: 10},
	}))

	list, err := GetInvoiceableOrders(7)
	require.NoError(t, err)
	require.Len(t, list, 2) // topup#2 + sub#9；不含他人/未支付/已占用
	keys := map[string]bool{}
	for _, o := range list {
		keys[o.OrderType+fmt.Sprint(o.OrderId)] = true
	}
	require.True(t, keys["topup2"])
	require.True(t, keys["subscription9"])
}
