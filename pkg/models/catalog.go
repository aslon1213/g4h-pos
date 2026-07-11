package models

import "time"

// Category is a node in the catalog category tree. ParentID is empty for a
// top-level category. Stored in the `categories` collection.
type Category struct {
	ID          string    `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	Name        string    `json:"name" bson:"name" gorm:"column:name"`
	Slug        string    `json:"slug" bson:"slug" gorm:"column:slug"`
	ParentID    string    `json:"parent_id" bson:"parent_id" gorm:"column:parent_id"`
	Description string    `json:"description" bson:"description" gorm:"column:description"`
	Image       string    `json:"image" bson:"image" gorm:"column:image"`
	SortOrder   int       `json:"sort_order" bson:"sort_order" gorm:"column:sort_order"`
	IsActive    bool      `json:"is_active" bson:"is_active" gorm:"column:is_active"`
	CreatedAt   time.Time `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt   time.Time `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`
}

func (Category) TableName() string { return "categories" }

// Brand / manufacturer used to filter the catalog. Stored in the `brands`
// collection (and also derivable from product manufacturer data).
type Brand struct {
	ID      string `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	Name    string `json:"name" bson:"name" gorm:"column:name"`
	Slug    string `json:"slug" bson:"slug" gorm:"column:slug"`
	Logo    string `json:"logo" bson:"logo" gorm:"column:logo"`
	Country string `json:"country" bson:"country" gorm:"column:country"`
}

func (Brand) TableName() string { return "brands" }

// CategoryInput is the body for admin create/update of a catalog category
// (POST/PUT /api/v1/admin/catalog/categories).
type CategoryInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	ParentID    string `json:"parent_id"`
	Description string `json:"description"`
	Image       string `json:"image"`
	SortOrder   int    `json:"sort_order"`
	IsActive    *bool  `json:"is_active"` // pointer so "false" is distinguishable from "unset" on update
}

// BrandInput is the body for admin create/update of a catalog brand
// (POST/PUT /api/v1/admin/catalog/brands).
type BrandInput struct {
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Logo    string `json:"logo"`
	Country string `json:"country"`
}

// BranchAvailability reports stock of a product at one location.
type BranchAvailability struct {
	BranchID string `json:"branch_id" bson:"branch_id"`
	Quantity int    `json:"quantity" bson:"quantity"`
	InStock  bool   `json:"in_stock" bson:"in_stock"`
}

// ProductListParams carries filter/sort/pagination options for the storefront
// product and catalog-search endpoints (bound from the query string).
type ProductListParams struct {
	Query    string  `query:"q"`
	Category string  `query:"category"`
	Brand    string  `query:"brand"`
	PriceMin float64 `query:"price_min"`
	PriceMax float64 `query:"price_max"`
	InStock  bool    `query:"in_stock"`
	Sort     string  `query:"sort"` // e.g. "price_asc", "price_desc", "newest"
	Page     int     `query:"page"`
	Count    int     `query:"count"`
}

// PagedProducts is the paginated list payload returned by product/search lists.
// Products are the full Product model (from product-models.go) so the storefront
// shares one product shape with the admin side; each is enriched with its
// catalog/review info by the storefront product repository on read.
type PagedProducts struct {
	Products []Product `json:"products" bson:"products"`
	Total    int64     `json:"total" bson:"total"`
	Page     int       `json:"page" bson:"page"`
	Count    int       `json:"count" bson:"count"`
}
