package models

import (
	"time"

	"github.com/google/uuid"
)

// ProductQuantityInfo represents the quantity and unit of measurement for a product
type ProductQuantityInfo struct {
	Quantity int32  `json:"quantity" bson:"quantity"` // Quantity of the product
	Unit     string `json:"unit" bson:"unit"`         // Unit of measurement (e.g. kg, pieces, etc)
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
	ID        string           `json:"id" bson:"id"`                 // Unique identifier for the place
	PlaceType ProductPlaceType `json:"place_type" bson:"place_type"` // Type of storage location
}
type ProductItemID string

// ProductQuantityDistribution tracks how much of a product is stored in each location
type ProductDistribution struct {
	ProductQuantityInfo              // Embedded quantity info
	Place               ProductPlace `json:"place" bson:"place"` // Location details
	Price               int32        `json:"price" bson:"price"` // Price of the product
}

type PriceDistribution struct {
	Price int32        `json:"price" bson:"price"` // Price of the product
	Place ProductPlace `json:"place" bson:"place"` // Place where the price is applied
}

// ManufacturerInfo contains details about the product manufacturer
type ManufacturerInfo struct {
	Name    string `json:"name" bson:"name"`       // Manufacturer name
	Country string `json:"country" bson:"country"` // Country of manufacture
	Address string `json:"address" bson:"address"` // Manufacturer address
	Phone   string `json:"phone" bson:"phone"`     // Contact phone number
	Email   string `json:"email" bson:"email"`     // Contact email
}

type IncomeHistory struct {
	Date       string       `json:"date" bson:"date"`               // Date of the income
	Price      int32        `json:"price" bson:"price"`             // Price of the product that was uploaded
	Quantity   int32        `json:"quantity" bson:"quantity"`       // Quantity of the product that was uploaded
	UploadedTo ProductPlace `json:"uploaded_to" bson:"uploaded_to"` // Place where the product was uploaded to
	SupplierID string       `json:"supplier_id" bson:"supplier_id"` // Supplier ID
}

// this is used to track every item of this product type.
type ProductItem struct {
	Expire time.Time `json:"expire" bson:"expire"` // Expire date of the product item
	Price  int32     `json:"price" bson:"price"`   // Price of the product item
}

type ProductBase struct {
	Name              string           `json:"name" bson:"name"`                               // Product name
	Description       string           `json:"description" bson:"description"`                 // Product description
	Manufacturer      ManufacturerInfo `json:"manufacturer" bson:"manufacturer"`               // Manufacturer details
	Category          []string         `json:"category" bson:"category"`                       // Product categories
	SKU               string           `json:"sku" bson:"sku"`                                 // Stock Keeping Unit
	MinimumStockAlert int32            `json:"minimum_stock_alert" bson:"minimum_stock_alert"` // Minimum stock alert
	GeneralIncomePrice float32            `json:"general_income_price" bson:"general_income_price"` // General income price of the product --- the price generally this item is bought from supplier
}

// Product represents a complete product entity with all its details
type Product struct {
	ID                   string                `json:"id" bson:"_id"` // Unique product identifier
	ProductBase          `bson:",inline"`      // Unique product identifier
	CreatedAt            time.Time             `json:"created_at" bson:"created_at"`                       // Creation timestamp
	UpdatedAt            time.Time             `json:"updated_at" bson:"updated_at"`                       // Last update timestamp
	QuantityDistribution []ProductDistribution `json:"quantity_distribution" bson:"quantity_distribution"` // Stock levels by location
	Images               []string              `json:"images" bson:"images"`                               // Product images (lins) saved to some S3
	IncomeHistory        []IncomeHistory       `json:"income_history" bson:"income_history"`               // Income history
}

func NewProduct(productBase *ProductBase) *Product {
	return &Product{
		ID:                   uuid.New().String(),
		ProductBase:          *productBase,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		QuantityDistribution: []ProductDistribution{},
		Images:               []string{},
		IncomeHistory:        []IncomeHistory{},
	}
}

type ProductOutput struct {
	Data  []Product `json:"data"`
	Error []Error   `json:"error"`
}

// ProductQueryParams defines the available search parameters for products
type ProductQueryParams struct {
	Name string `query:"name"` // Filter by name
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
