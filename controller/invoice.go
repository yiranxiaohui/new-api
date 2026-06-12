package controller

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type createInvoiceRequest struct {
	OrderKeys []struct {
		Type string `json:"type"`
		Id   int    `json:"id"`
	} `json:"order_keys"`
	TitleType     int    `json:"title_type"`
	TitleName     string `json:"title_name"`
	TaxNo         string `json:"tax_no"`
	Email         string `json:"email"`
	Remark        string `json:"remark"`
	SaveAsDefault bool   `json:"save_as_default"`
}

// GetInvoiceableOrders GET /api/user/invoice/orders —— 同时返回默认抬头
func GetInvoiceableOrders(c *gin.Context) {
	userId := c.GetInt("id")
	orders, err := model.GetInvoiceableOrders(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var title *dto.InvoiceTitle
	if user, err := model.GetUserById(userId, false); err == nil {
		setting := user.GetSetting()
		title = setting.InvoiceTitle
	}
	common.ApiSuccess(c, gin.H{"orders": orders, "default_title": title})
}

// GetUserInvoiceList GET /api/user/invoice/self
func GetUserInvoiceList(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	list, total, err := model.GetUserInvoices(userId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(list)
	common.ApiSuccess(c, pageInfo)
}

// CreateInvoice POST /api/user/invoice
func CreateInvoice(c *gin.Context) {
	userId := c.GetInt("id")
	var req createInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(req.OrderKeys) == 0 {
		common.ApiErrorMsg(c, "请选择要开票的订单")
		return
	}
	if req.TitleType != model.InvoiceTitleTypePersonal && req.TitleType != model.InvoiceTitleTypeCompany {
		common.ApiErrorMsg(c, "抬头类型无效")
		return
	}
	if req.TitleName == "" || req.Email == "" {
		common.ApiErrorMsg(c, "抬头名称与接收邮箱不能为空")
		return
	}
	if req.TitleType == model.InvoiceTitleTypeCompany && req.TaxNo == "" {
		common.ApiErrorMsg(c, "企业抬头必须填写税号")
		return
	}
	// 服务端核验：仅允许选择当前可开票订单（归属本人/success/未占用），占用竞态由唯一索引兜底
	available, err := model.GetInvoiceableOrders(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	availMap := make(map[string]*model.InvoiceableOrder, len(available))
	for _, o := range available {
		availMap[o.OrderType+"#"+strconv.Itoa(o.OrderId)] = o
	}
	orders := make([]*model.InvoiceOrder, 0, len(req.OrderKeys))
	for _, k := range req.OrderKeys {
		o, ok := availMap[k.Type+"#"+strconv.Itoa(k.Id)]
		if !ok {
			common.ApiErrorMsg(c, fmt.Sprintf("订单不可开票或不存在: %s#%d", k.Type, k.Id))
			return
		}
		orders = append(orders, &model.InvoiceOrder{OrderType: o.OrderType, OrderId: o.OrderId,
			TradeNo: o.TradeNo, Money: o.Money})
	}
	inv := &model.Invoice{UserId: userId, TitleType: req.TitleType, TitleName: req.TitleName,
		TaxNo: req.TaxNo, Email: req.Email, Remark: req.Remark}
	if err := model.CreateInvoiceWithOrders(inv, orders); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.SaveAsDefault {
		saveDefaultInvoiceTitle(userId, &req)
	}
	common.ApiSuccess(c, inv)
}

func saveDefaultInvoiceTitle(userId int, req *createInvoiceRequest) {
	user, err := model.GetUserById(userId, true)
	if err != nil {
		common.SysError("failed to load user for invoice title: " + err.Error())
		return
	}
	setting := user.GetSetting()
	setting.InvoiceTitle = &dto.InvoiceTitle{TitleType: req.TitleType,
		TitleName: req.TitleName, TaxNo: req.TaxNo, Email: req.Email}
	user.SetSetting(setting)
	if err := user.Update(false); err != nil {
		common.SysError("failed to save default invoice title: " + err.Error())
	}
}

// CancelInvoice DELETE /api/user/invoice/:id
func CancelInvoice(c *gin.Context) {
	userId := c.GetInt("id")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.CancelInvoice(id, userId); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// DownloadInvoiceFileUser GET /api/user/invoice/:id/file
func DownloadInvoiceFileUser(c *gin.Context) {
	userId := c.GetInt("id")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	inv, err := model.GetInvoiceById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if inv.UserId != userId {
		common.ApiErrorMsg(c, "无权访问该发票")
		return
	}
	serveInvoiceFile(c, id)
}

func serveInvoiceFile(c *gin.Context, invoiceId int) {
	f, err := model.GetInvoiceFile(invoiceId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	safeFilename := strings.NewReplacer("\"", "", "\r", "", "\n", "").Replace(f.Filename)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		safeFilename, url.PathEscape(safeFilename)))
	c.Data(200, f.MimeType, f.Data)
}
