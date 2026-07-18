package activities_repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type ActivityType string

const (
	ActivityTypeLogin             ActivityType = "login_success"
	ActivityTypeLoginFailed       ActivityType = "login_failed"
	ActivityTypeLogout            ActivityType = "logout_success"
	ActivityTypeLogoutFailed      ActivityType = "logout_failed"
	ActivityTypeRegister          ActivityType = "register_success"
	ActivityTypeRegisterFailed    ActivityType = "register_failed"
	ActivityTypeCreateTransaction ActivityType = "create_transaction"
	ActivityTypeCreateJournal     ActivityType = "create_journal"
	ActivityTypeCloseJournal      ActivityType = "close_journal"
	ActivityTypeCreateSupplier    ActivityType = "create_supplier"
	ActivityTypeCreateProduct     ActivityType = "create_product"
	ActivityTypeEditTransaction   ActivityType = "edit_transaction"
	ActivityTypeEditSupplier      ActivityType = "edit_supplier"
	ActivityTypeEditProduct       ActivityType = "edit_product"
	ActivityTypeDeleteTransaction ActivityType = "delete_transaction"
	ActivityTypeReopenJournal     ActivityType = "reopen_journal"
	ActivityTypeDeleteSupplier    ActivityType = "delete_supplier"
	ActivityTypeDeleteProduct     ActivityType = "delete_product"
	ActivityTypeCloseSalesSession ActivityType = "close_sales_session"
	ActivityTypeOpenSalesSession  ActivityType = "open_sales_session"
	ActivityTypeProductIncome     ActivityType = "product_income"
	ActivityTypeProductTransfer   ActivityType = "product_transfer"
	ActivityTypeCreateOperation   ActivityType = "create_operation"
	ActivityTypeEditOperation     ActivityType = "edit_operation"
	ActivityTypeDeleteOperation   ActivityType = "delete_operation"
	ActivityTypeCreateFinance     ActivityType = "create_finance"
	ActivityTypeEditFinance       ActivityType = "edit_finance"
	ActivityTypeDeleteFinance     ActivityType = "delete_finance"
	ActivityTypeCreateCategory    ActivityType = "create_category"
	ActivityTypeEditCategory      ActivityType = "edit_category"
	ActivityTypeDeleteCategory    ActivityType = "delete_category"
	ActivityTypeCreateBrand       ActivityType = "create_brand"
	ActivityTypeEditBrand         ActivityType = "edit_brand"
	ActivityTypeDeleteBrand       ActivityType = "delete_brand"
	ActivityTypeHandoffClaim      ActivityType = "handoff_claim"
	ActivityTypeHandoffCheckout   ActivityType = "handoff_checkout"
)

type Activity struct {
	ID     uint            `gorm:"primaryKey"`
	UserID string          `gorm:"column:user_id"`
	Action ActivityType    `gorm:"column:action"`
	Data   json.RawMessage `gorm:"column:data;type:json" swaggertype:"object"`
	IP     string          `gorm:"column:ip"`
	Date   time.Time       `gorm:"column:date"`
	Status int             `gorm:"column:status"`
}

func (Activity) TableName() string {
	return "activities"
}

// ActivitiesRepo provides methods to record staff/user activities in the database.
type ActivitiesRepo struct {
	db *gorm.DB
}

// New returns a new ActivitiesRepo for a given gorm.DB.
func New(db *gorm.DB) *ActivitiesRepo {
	return &ActivitiesRepo{db: db}
}

// Record records a generic activity.
// 'activity' must be of type Activity as defined above, not from middleware.
func (r *ActivitiesRepo) Record(ctx context.Context, activity Activity) error {
	return r.db.WithContext(ctx).Create(&activity).Error
}

// LogActivity records an activity with detailed fields.
func (r *ActivitiesRepo) LogActivity(user string, action ActivityType, data interface{}, ip string, status int) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	act := Activity{
		UserID: user,
		Action: action,
		Data:   raw,
		IP:     ip,
		Date:   time.Now(),
		Status: status,
	}
	return r.db.Create(&act).Error
}

// LogActivityWithCtx allows logging activity using Fiber context (for web).
// Pass in a Fiber context from a handler.
// Example: repo.LogActivityWithCtx(c, ActivityTypeLogin, userData)
func (r *ActivitiesRepo) LogActivityWithCtx(ctx *fiber.Ctx, action ActivityType, data interface{}) error {
	userVal := ctx.Locals("user_id")
	var user string
	if userVal != nil {
		user, _ = userVal.(string)
	}
	ip := ctx.IP()
	status := 0
	resp := ctx.Response()
	if resp != nil {
		status = resp.StatusCode()
	}
	return r.LogActivity(user, action, data, ip, status)
}

// ListActivitiesOfUser returns all activities performed by a user, optionally with pagination (limit, offset).
// If limit <= 0, returns all activities for the user.
func (r *ActivitiesRepo) ListActivitiesOfUser(ctx context.Context, userID string, limit, offset int) ([]Activity, error) {
	var activities []Activity
	query := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("date DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	err := query.Find(&activities).Error
	return activities, err
}

// ListActivitiesHandler is a Fiber handler to list activities for the authenticated user with optional pagination via query params.
func (r *ActivitiesRepo) ListActivities(c *fiber.Ctx) ([]Activity, error) {
	user := c.Locals("user_id")
	if user == nil {
		return nil, fmt.Errorf("user is not authenticated")
	}
	userID, ok := user.(string)
	if !ok {
		return nil, fmt.Errorf("User is not authenticated properly")
	}

	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)

	activities, err := r.ListActivitiesOfUser(c.Context(), userID, limit, offset)
	if err != nil {
		return nil, err
	}

	return activities, nil
}

// ListActivitiesHandler is a Fiber handler to list activities for the authenticated user with optional pagination via query params.
func (r *ActivitiesRepo) ListRecentActivities(c context.Context, limit int, offset int) ([]Activity, error) {

	var activities []Activity
	query := r.db.WithContext(c).Order("date DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	err := query.Find(&activities).Error
	return activities, err
}
