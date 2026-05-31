package models

import (
	"encoding/json"
	"time"

	"github.com/aslon1213/g4h_pos_erp/platform/cache"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type SalesSession struct {
	ID        string                      `json:"id" bson:"_id"`
	BranchID  string                      `json:"branch_id" bson:"branch_id"`
	CreatedAt time.Time                   `json:"created_at" bson:"created_at"`
	Products  map[string]SalesSessionItem `json:"product_items" bson:"product_items"` // product which are going to be sold
}

type SalesSessionItem struct {
	Quantity int   `json:"quantity" bson:"quantity"`
	Price    int32 `json:"price" bson:"price"`
}

func NewSalesSession(branchID string, cache *cache.Cache) (*SalesSession, error) {
	log.Info().Str("branch_id", branchID).Msg("Creating new sales session")

	session := &SalesSession{
		ID:        uuid.New().String(),
		BranchID:  branchID,
		CreatedAt: time.Now(),
		Products:  map[string]SalesSessionItem{},
	}

	// Serialize session to JSON
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal session")
		return nil, err
	}

	// Add to cache with 1 hour expiry
	err = cache.RedisClient.Set("sales_session:"+session.ID, sessionJSON, 1*time.Hour).Err()
	if err != nil {
		log.Error().Err(err).Msg("Failed to cache session")
		return nil, err
	}

	log.Debug().Str("session_id", session.ID).Msg("Added session to cache")
	return session, nil
}

func (s *SalesSession) DeleteSession(cache *cache.Cache) error {
	log.Info().Str("session_id", s.ID).Msg("Deleting sales session")
	err := cache.RedisClient.Del("sales_session:" + s.ID).Err()
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete session from cache")
		return err
	}
	return nil
}

func GetSalesSession(id string, cache *cache.Cache) (*SalesSession, error) {
	log.Info().Str("session_id", id).Msg("Getting sales session")

	// Get session from cache
	sessionJSON, err := cache.RedisClient.Get("sales_session:" + id).Result()
	if err != nil {
		log.Error().Err(err).Str("session_id", id).Msg("Failed to get session from cache")
		return nil, err
	}

	// Deserialize JSON to session struct
	session := &SalesSession{}
	err = json.Unmarshal([]byte(sessionJSON), session)
	if err != nil {
		log.Error().Err(err).Str("session_id", id).Msg("Failed to unmarshal session data")
		return nil, err
	}

	return session, nil
}

func GetSalesSessionByBranchID(branchID string, cache *cache.Cache) ([]*SalesSession, error) {
	log.Info().Str("branch_id", branchID).Msg("Getting sales session by branch ID")
	var cursor uint64
	var keys []string

	// Step 1: Scan all keys matching sales_session:*
	for {
		var err error
		var k []string
		k, cursor, err = cache.RedisClient.Scan(cursor, "sales_session:*", 100).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, k...)
		if cursor == 0 {
			break
		}
	}

	results := []*SalesSession{}

	// Step 2: Fetch each session data (assuming you use Redis hashes)
	for _, key := range keys {
		data, err := cache.RedisClient.Get(key).Result()
		if err != nil {
			return nil, err
		}
		session := &SalesSession{}
		err = json.Unmarshal([]byte(data), session)
		if err != nil {
			return nil, err
		}
		results = append(results, session)
	}

	output_sessions := []*SalesSession{}
	for _, session := range results {
		if session.BranchID == branchID {
			output_sessions = append(output_sessions, session)
		}
	}

	return output_sessions, nil
}

func (s *SalesSession) AddProductItem(productID string, quantity int, price int32, cache *cache.Cache) error {
	log.Info().Str("session_id", s.ID).Str("product_id", productID).Int("quantity", quantity).Int32("price", price).Msg("Adding product item to session")

	if item, ok := s.Products[productID]; ok {
		s.Products[productID] = SalesSessionItem{
			Quantity: item.Quantity + quantity,
			Price:    item.Price,
		}
		log.Debug().Str("product_id", productID).Int("new_quantity", item.Quantity+quantity).Msg("Updated existing product quantity")
	} else {
		s.Products[productID] = SalesSessionItem{
			Quantity: quantity,
			Price:    price,
		}
		log.Debug().Str("product_id", productID).Msg("Added new product to session")
	}

	// Serialize updated session
	sessionJSON, err := json.Marshal(s)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal updated session")
		return err
	}

	// Update cache with 1 hour expiry
	err = cache.RedisClient.Set("sales_session:"+s.ID, sessionJSON, 1*time.Hour).Err()
	if err != nil {
		log.Error().Err(err).Msg("Failed to update session in cache")
		return err
	}
	return nil
}

func (s *SalesSession) RemoveProductItem(productID string, cache *cache.Cache) error {
	log.Info().Str("session_id", s.ID).Str("product_id", productID).Msg("Removing product item from session")
	delete(s.Products, productID)

	// Serialize updated session
	sessionJSON, err := json.Marshal(s)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal updated session")
		return err
	}

	// Update cache with 1 hour expiry
	err = cache.RedisClient.Set("sales_session:"+s.ID, sessionJSON, 1*time.Hour).Err()
	if err != nil {
		log.Error().Err(err).Msg("Failed to update session in cache")
		return err
	}
	return nil
}
