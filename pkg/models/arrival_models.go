package models

import (
	"time"
)

type ProductProposal struct {
	ID        string    `json:"id" bson:"_id,omitempty" gorm:"column:id;primaryKey"`
	Name      string    `json:"name" bson:"name" gorm:"column:name"`
	Date      time.Time `json:"date" bson:"date" gorm:"column:date"`
	Branch    string    `json:"branch" bson:"branch" gorm:"column:branch"`
	Fulfilled bool      `json:"fulfilled" bson:"fulfilled" gorm:"column:fulfilled"`
	ImageFile *string   `json:"image_file" bson:"image_file,omitempty" gorm:"column:image_file"`
}

func (ProductProposal) TableName() string { return "proposals" }
