package models

import "time"

// Category is a node in the catalog category tree. ParentID is empty for a
// top-level category. Stored in the `categories` collection.
type Category struct {
	ID          string    `json:"id" bson:"_id"`
	Name        string    `json:"name" bson:"name"`
	Slug        string    `json:"slug" bson:"slug"`
	ParentID    string    `json:"parent_id" bson:"parent_id"`
	Description string    `json:"description" bson:"description"`
	Image       string    `json:"image" bson:"image"`
	SortOrder   int       `json:"sort_order" bson:"sort_order"`
	IsActive    bool      `json:"is_active" bson:"is_active"`
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" bson:"updated_at"`
}

// Brand / manufacturer used to filter the catalog. Stored in the `brands`
// collection (and also derivable from product manufacturer data).
type Brand struct {
	ID      string `json:"id" bson:"_id"`
	Name    string `json:"name" bson:"name"`
	Slug    string `json:"slug" bson:"slug"`
	Logo    string `json:"logo" bson:"logo"`
	Country string `json:"country" bson:"country"`
}

// StoreProduct is the public, read-only projection of a product for the
// storefront. It maps onto the existing `products` collection (the staff-side
// model is repository.Product) but exposes only customer-facing fields.
type StoreProduct struct {
	ID          string    `json:"id" bson:"_id"`
	Name        string    `json:"name" bson:"name"`
	Description string    `json:"description" bson:"description"`
	SKU         string    `json:"sku" bson:"sku"`
	Category    []string  `json:"category" bson:"category"`
	Images      []string  `json:"images" bson:"images"`
	Price       float64   `json:"price" bson:"price"`
	InStock     bool      `json:"in_stock" bson:"in_stock"`
	Rating      float64   `json:"rating" bson:"rating"`
	ReviewCount int       `json:"review_count" bson:"review_count"`
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" bson:"updated_at"`
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
type PagedProducts struct {
	Products []StoreProduct `json:"products" bson:"products"`
	Total    int64          `json:"total" bson:"total"`
	Page     int            `json:"page" bson:"page"`
	Count    int            `json:"count" bson:"count"`
}
