package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
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
)

type Activity struct {
	UserID string       `bson:"user_id"`
	Action ActivityType `bson:"action"`
	Data   interface{}  `bson:"data"`
	IP     string       `bson:"ip"`
	Date   time.Time    `bson:"date"`
	Status int          `bson:"status"`
}

// func SetActionType(ctx *fiber.Ctx, action ActivityType) {
// 	log.Debug().Str("action", string(action)).Msg("SetActionType called")
// 	ctx.Locals(
// 		"action",
// 		action,
// 	)
// }

// func SetUser(ctx *fiber.Ctx, user string) {
// 	log.Debug().Str("user", user).Msg("SetUser called")
// 	ctx.Locals(
// 		"user",
// 		user,
// 	)
// }

// func SetData(ctx *fiber.Ctx, data interface{}) {
// 	log.Debug().Interface("data", data).Msg("SetData called")
// 	ctx.Locals(
// 		"data",
// 		data,
// 	)
// }

func LogActivityWithCtx(ctx *fiber.Ctx, action ActivityType, data interface{}, collection *mongo.Collection) {
	log.Debug().Msg("LogActivityWithCtx called")

	user, ok := ctx.Locals("user").(string)
	if !ok {
		log.Error().Msg("User not found in context")
		user = "unknown"
	}

	LogActivity(user, action, data, ctx.IP(), ctx.Response().StatusCode(), collection)
}

func LogActivity(user string, action ActivityType, data interface{}, ip string, status int, collection *mongo.Collection) {
	log.Debug().Msg("LogActivity called")
	activity := Activity{
		UserID: user,
		Action: action,
		Data:   data,
		IP:     ip,
		Status: status,
		Date:   time.Now(),
	}
	_, err := collection.InsertOne(context.Background(), activity)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert activity")
	}
}

func RecordActicity(ctx context.Context, activity Activity, activities *mongo.Collection) error {
	_, err := activities.InsertOne(ctx, activity)
	if err != nil {
		return err
	}
	return nil
}
