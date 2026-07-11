package models

import "time"

// Review is a customer's rating + comment on a product. Stored in the `reviews`
// collection.
type Review struct {
	ID           string    `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	ProductID    string    `json:"product_id" bson:"product_id" gorm:"column:product_id"`
	CustomerID   string    `json:"customer_id" bson:"customer_id" gorm:"column:customer_id"`
	CustomerName string    `json:"customer_name" bson:"customer_name" gorm:"column:customer_name"`
	Rating       int       `json:"rating" bson:"rating" gorm:"column:rating"` // 1..5
	Title        string    `json:"title" bson:"title" gorm:"column:title"`
	Body         string    `json:"body" bson:"body" gorm:"column:body"`
	HelpfulVotes int       `json:"helpful_votes" bson:"helpful_votes" gorm:"column:helpful_votes"`
	Voters       []string  `json:"-" bson:"voters" gorm:"-"` // customer ids who voted (dedupe)
	CreatedAt    time.Time `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt    time.Time `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`
}

func (Review) TableName() string { return "reviews" }

type ReviewVote struct {
	ReviewID   string `gorm:"column:review_id;primaryKey"`
	CustomerID string `gorm:"column:customer_id;primaryKey"`
}

func (ReviewVote) TableName() string { return "review_votes" }

// PagedReviews is the paginated review list payload.
type PagedReviews struct {
	Reviews []Review `json:"reviews" bson:"reviews"`
	Total   int64    `json:"total" bson:"total"`
	Page    int      `json:"page" bson:"page"`
	Count   int      `json:"count" bson:"count"`
}

// CreateReviewInput is the body for POST /api/v1/store/products/{id}/reviews.
type CreateReviewInput struct {
	Rating int    `json:"rating"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// UpdateReviewInput is the body for PUT /api/v1/store/reviews/{id}.
type UpdateReviewInput struct {
	Rating int    `json:"rating"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// VoteReviewInput is the body for POST /api/v1/store/reviews/{id}/vote.
type VoteReviewInput struct {
	Helpful bool `json:"helpful"`
}
