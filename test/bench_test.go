package test

import (
	"testing"

	"github.com/aslon1213/g4h_pos_erp/pkg/configs"
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/test/client"
	"github.com/rs/zerolog/log"
)

func BenchmarkSessionOperations(b *testing.B) {
	log.Info().Msg("Starting session operations benchmark")

	// init config
	log.Debug().Msg("Loading config")
	config, err := configs.LoadConfig("../")
	if err != nil {
		log.Error().Err(err).Msg("Failed to load config")
		b.Fatal(err)
	}

	// init client
	log.Debug().Msg("Initializing client")
	client := client.NewClient(config.Server.Host, config.Server.Port, config.DB.Username, config.DB.Password)

	// get branches
	log.Debug().Msg("Getting all branches")
	_, data, err := client.GetAllBranches()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get branches")
		b.Fatal(err)
	}
	branches := data.Data
	// choose the first branch
	branch := branches[0]
	log.Debug().Str("branchID", branch.BranchID).Msg("Selected branch")

	// get products
	log.Debug().Str("branchID", branch.BranchID).Msg("Querying products for branch")
	_, data_2, err := client.QueryProducts(
		&models.ProductQueryParams{
			BranchID: branch.BranchID,
		},
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query products")
		b.Fatal(err)
	}
	products := data_2.Data
	log.Debug().Int("productCount", len(products)).Msg("Retrieved products")

	// create session
	log.Debug().Str("branchID", branch.BranchID).Msg("Creating new session")
	session, err := client.NewSession(branch.BranchID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create session")
		b.Fatal(err)
	}
	log.Debug().Str("sessionID", session.ID).Msg("Session created")

	// add products
	log.Debug().Msg("Adding products to session")
	for _, product := range products {
		_, err := client.AddProductToSession(session.ID, product.ID, 1)
		if err != nil {
			log.Error().Err(err).Str("productID", product.ID).Msg("Failed to add product to session")
			b.Fatal(err)
		}
		log.Debug().Str("productID", product.ID).Msg("Added product to session")
	}

	// get session
	log.Debug().Str("sessionID", session.ID).Msg("Getting session details")
	_, err = client.GerSalesSession(session.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get session")
		b.Fatal(err)
	}
	log.Info().Msg("Benchmark completed successfully")
}
