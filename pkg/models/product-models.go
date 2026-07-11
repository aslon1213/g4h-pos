package models

import (
	"time"

	"github.com/google/uuid"
)

// ProductQuantityInfo represents the quantity and unit of measurement for a product
type ProductQuantityInfo struct {
	Quantity int32  `json:"quantity" bson:"quantity" gorm:"column:quantity"` // Quantity of the product
	Unit     string `json:"unit" bson:"unit" gorm:"column:unit"`             // Unit of measurement (e.g. kg, pieces, etc)
}

// ProductPlaceType defines where products can be stored
type ProductPlaceType string

// Types of places where products can be stored
const (
	ProductPlaceTypeBranch    ProductPlaceType = "branch"    // Store branch location
	ProductPlaceTypeWarehouse ProductPlaceType = "warehouse" // Central warehouse location
)

// ProductPlace represents a physical location where products are stored
type ProductPlace struct {
	ID        string           `json:"id" bson:"id" gorm:"column:id"`                   // Unique identifier for the place
	PlaceType ProductPlaceType `json:"place_type" bson:"place_type" gorm:"column:type"` // Type of storage location
}
type ProductItemID string

// ProductQuantityDistribution tracks how much of a product is stored in each location
type ProductDistribution struct {
	ID                  uint              `json:"-" bson:"-" gorm:"column:id;primaryKey"`
	ProductID           string            `json:"-" bson:"-" gorm:"column:product_id"`
	ProductQuantityInfo `gorm:"embedded"` // Embedded quantity info
	Place               ProductPlace      `json:"place" bson:"place" gorm:"embedded;embeddedPrefix:place_"` // Location details
	Price               int32             `json:"price" bson:"price" gorm:"column:price"`                   // Price of the product
}

func (ProductDistribution) TableName() string { return "product_stock" }

type PriceDistribution struct {
	Price int32        `json:"price" bson:"price"` // Price of the product
	Place ProductPlace `json:"place" bson:"place"` // Place where the price is applied
}

// ManufacturerInfo contains details about the product manufacturer
type ManufacturerInfo struct {
	Name    string `json:"name" bson:"name" gorm:"column:name"`          // Manufacturer name
	Country string `json:"country" bson:"country" gorm:"column:country"` // Country of manufacture
	Address string `json:"address" bson:"address" gorm:"column:address"` // Manufacturer address
	Phone   string `json:"phone" bson:"phone" gorm:"column:phone"`       // Contact phone number
	Email   string `json:"email" bson:"email" gorm:"column:email"`       // Contact email
}

type IncomeHistory struct {
	ID         uint         `json:"-" bson:"-" gorm:"column:id;primaryKey"`
	ProductID  string       `json:"-" bson:"-" gorm:"column:product_id"`
	Date       string       `json:"date" bson:"date" gorm:"column:date"`                                  // Date of the income
	Price      int32        `json:"price" bson:"price" gorm:"column:price"`                               // Price of the product that was uploaded
	Quantity   int32        `json:"quantity" bson:"quantity" gorm:"column:quantity"`                      // Quantity of the product that was uploaded
	UploadedTo ProductPlace `json:"uploaded_to" bson:"uploaded_to" gorm:"embedded;embeddedPrefix:place_"` // Place where the product was uploaded to
	SupplierID string       `json:"supplier_id" bson:"supplier_id" gorm:"column:supplier_id"`             // Supplier ID
}

func (IncomeHistory) TableName() string { return "product_income_history" }

// this is used to track every item of this product type.
type ProductItem struct {
	Expire time.Time `json:"expire" bson:"expire"` // Expire date of the product item
	Price  int32     `json:"price" bson:"price"`   // Price of the product item
}

type ProductBase struct {
	Name               string           `json:"name" bson:"name" gorm:"column:name"`                                                 // Product name
	Description        string           `json:"description" bson:"description" gorm:"column:description"`                            // Product description
	Manufacturer       ManufacturerInfo `json:"manufacturer" bson:"manufacturer" gorm:"embedded;embeddedPrefix:manufacturer_"`       // Manufacturer details
	Category           []string         `json:"category" bson:"category" gorm:"-"`                                                   // Product category ids (referencing the `categories` catalog collection)
	BrandID            string           `json:"brand_id" bson:"brand_id" gorm:"column:brand_id"`                                     // Brand id (referencing the `brands` catalog collection)
	SKU                string           `json:"sku" bson:"sku" gorm:"column:sku"`                                                    // Stock Keeping Unit
	MinimumStockAlert  int32            `json:"minimum_stock_alert" bson:"minimum_stock_alert" gorm:"column:minimum_stock_alert"`    // Minimum stock alert
	GeneralIncomePrice float32          `json:"general_income_price" bson:"general_income_price" gorm:"column:general_income_price"` // General income price of the product --- the price generally this item is bought from supplier
}

// Product represents a complete product entity with all its details.
//
// Rating and ReviewCount are denormalized review aggregates maintained by the
// review repository (recomputed whenever a review is created/updated/deleted) so
// the storefront can list/sort products without joining the reviews collection.
//
// Categories, Brand and Reviews are NOT stored (bson:"-"); they are catalog /
// review info *attached on read* by the storefront product repository when a
// product is fetched for the browse surface.
type Product struct {
	ID                   string                           `json:"id" bson:"_id" gorm:"column:id;primaryKey"` // Unique product identifier
	ProductBase          `bson:",inline" gorm:"embedded"` // Unique product identifier
	CreatedAt            time.Time                        `json:"created_at" bson:"created_at" gorm:"column:created_at"`       // Creation timestamp
	UpdatedAt            time.Time                        `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`       // Last update timestamp
	QuantityDistribution []ProductDistribution            `json:"quantity_distribution" bson:"quantity_distribution" gorm:"-"` // Stock levels by location
	Images               []string                         `json:"images" bson:"images" gorm:"column:images;serializer:json"`   // Product images (lins) saved to some S3
	IncomeHistory        []IncomeHistory                  `json:"income_history" bson:"income_history" gorm:"-"`               // Income history
	Rating               float64                          `json:"rating" bson:"rating" gorm:"column:rating"`                   // Average review rating (0..5), denormalized
	ReviewCount          int                              `json:"review_count" bson:"review_count" gorm:"column:review_count"` // Number of reviews, denormalized

	// Attached on read for the storefront (never persisted on the product doc).
	Categories []Category `json:"categories,omitempty" bson:"-" gorm:"-"` // Resolved catalog info for Category ids
	Brand      *Brand     `json:"brand,omitempty" bson:"-" gorm:"-"`      // Resolved catalog info for BrandID
	Reviews    []Review   `json:"reviews,omitempty" bson:"-" gorm:"-"`    // Recent reviews (product detail only)
}

func (Product) TableName() string { return "products" }

func NewProduct(productBase *ProductBase) *Product {
	return &Product{
		ID:                   uuid.New().String(),
		ProductBase:          *productBase,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		QuantityDistribution: []ProductDistribution{},
		Images:               []string{},
		IncomeHistory:        []IncomeHistory{},
		Rating:               0,
		ReviewCount:          0,
	}
}

// ProductQueryParams defines the available search parameters for products
type ProductQueryParams struct {
	Name     string  `query:"name"`      // Filter by name
	BranchID string  `query:"branch_id"` // Filter by branch
	Category string  `query:"category"`  // Filter by category
	SKU      string  `query:"sku"`       // Filter by SKU
	PriceMin float64 `query:"price_min"` // Minimum selling price
	PriceMax float64 `query:"price_max"` // Maximum selling price

}

// Logic for handling products
// Every product has a unique ID internally for better product management
// when a product comes to some Place, quantity distribution is updated
// Product quantity is decreased only when a product is sold.
// Product has supplier income history, which is used to calculate the cost price of the product and also see how price varies from supplier to supplier and what is inflation rate.
// Product can be trasferred, received, shrink, expired, etc.
// Product will have a ID for every item of this product type. For better tracking of expire date, transfer, etc.
// Discounts can be applied to the product. ----- this is later
