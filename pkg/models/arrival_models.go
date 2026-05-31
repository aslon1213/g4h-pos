package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type ProductProposal struct {
	ID        bson.ObjectID `json:"id" bson:"_id,omitempty"`
	Name      string        `json:"name" bson:"name"`
	Date      time.Time     `json:"date" bson:"date"`
	Branch    string        `json:"branch" bson:"branch"`
	Fulfilled bool          `json:"fulfilled" bson:"fulfilled"`
	ImageFile *string       `json:"image_file" bson:"image_file,omitempty"`
}
