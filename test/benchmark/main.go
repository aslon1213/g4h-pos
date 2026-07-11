package main

func SessionOperations() ([]float64, error) { // return time taken for every request
	// 	log.Info().Msg("Starting session operations benchmark")
	// 	request_times := []float64{}

	// 	// init config
	// 	log.Debug().Msg("Loading config")
	// 	config, err := configs.LoadConfig(".")
	// 	if err != nil {
	// 		log.Error().Err(err).Msg("Failed to load config")
	// 		return request_times, err
	// 	}

	// 	// init client
	// 	log.Debug().Msg("Initializing client")
	// 	client := client.NewClient(config.Server.Host, config.Server.Port, config.DB.Username, config.DB.Password)

	// 	// get branches

	// 	log.Debug().Msg("Getting all branches")
	// 	start := time.Now()
	// 	_, data, err := client.GetAllBranches()
	// 	if err != nil {
	// 		log.Error().Err(err).Msg("Failed to get branches")
	// 		return request_times, err
	// 	}
	// 	request_times = append(request_times, float64(time.Since(start).Milliseconds()))
	// 	branches := data.Data
	// 	// choose the first branch
	// 	branch := branches[0]
	// 	log.Debug().Str("branchID", branch.BranchID).Msg("Selected branch")

	// 	// get products
	// 	log.Debug().Str("branchID", branch.BranchID).Msg("Querying products for branch")
	// 	start = time.Now()
	// 	_, data_2, err := client.QueryProducts(
	// 		&models.ProductQueryParams{
	// 			BranchID: branch.BranchID,
	// 		},
	// 	)
	// 	if err != nil {
	// 		log.Error().Err(err).Msg("Failed to query products")
	// 		return request_times, err
	// 	}
	// 	request_times = append(request_times, float64(time.Since(start).Milliseconds()))
	// 	products := data_2.Data
	// 	log.Debug().Int("productCount", len(products)).Msg("Retrieved products")

	// 	// create session
	// 	log.Debug().Str("branchID", branch.BranchID).Msg("Creating new session")
	// 	start = time.Now()
	// 	session, err := client.NewSession(branch.BranchID)
	// 	if err != nil {
	// 		log.Error().Err(err).Msg("Failed to create session")
	// 		return request_times, err
	// 	}
	// 	log.Debug().Str("sessionID", session.ID).Msg("Session created")
	// 	request_times = append(request_times, float64(time.Since(start).Milliseconds()))
	// 	// add products
	// 	log.Debug().Msg("Adding products to session")
	// 	start = time.Now()
	// 	for _, product := range products {
	// 		_, err := client.AddProductToSession(session.ID, product.ID, 1)
	// 		if err != nil {
	// 			log.Error().Err(err).Str("productID", product.ID).Msg("Failed to add product to session")
	// 			return request_times, err
	// 		}
	// 		log.Debug().Str("productID", product.ID).Msg("Added product to session")
	// 	}
	// 	request_times = append(request_times, float64(time.Since(start).Milliseconds()))
	// 	// get session
	// 	log.Debug().Str("sessionID", session.ID).Msg("Getting session details")
	// 	start = time.Now()
	// 	session, err = client.GerSalesSession(session.ID)
	// 	if err != nil {
	// 		log.Error().Err(err).Msg("Failed to get session")
	// 		return request_times, err
	// 	}
	// 	request_times = append(request_times, float64(time.Since(start).Milliseconds()))
	// 	log.Info().Msg("Benchmark completed successfully")
	// 	return request_times, nil
	// }

	// func main() {

	// 	app := app.New()
	// 	// get all keys
	// 	keys, err := app.Cache.RedisClient.Keys("sales_session:*").Result()
	// 	if err != nil {
	// 		log.Error().Err(err).Msg("Failed to get keys")
	// 		return
	// 	}

	// 	// delete all sessions
	// 	app.Cache.RedisClient.Del(keys...)
	// 	os.Exit(0)

	// 	concurrency := 100
	// 	duration := 30 * time.Second

	// 	var wg sync.WaitGroup
	// 	ticker := time.NewTicker(time.Second / time.Duration(concurrency)) // ~1ms interval
	// 	defer ticker.Stop()

	// 	timeout := time.After(duration)

	// 	time_taken := []float64{}
	// 	error_count := 0

	// 	for {
	// 		select {
	// 		case <-timeout:
	// 			wg.Wait()
	// 			fmt.Println("Load test completed")
	// 			avg_time := 0.0
	// 			for _, t := range time_taken {
	// 				avg_time += t
	// 			}
	// 			avg_time /= float64(len(time_taken))
	// 			log.Info().Float64("avg_time", avg_time).Msg("Average time taken")
	// 			log.Info().Int("total_requests", len(time_taken)).Msg("Total requests")
	// 			log.Info().Int("error_count", error_count).Msg("Error count")
	// 			return
	// 		case <-ticker.C:
	// 			wg.Add(1)
	// 			go func() {
	// 				defer wg.Done()
	// 				request_times, err := SessionOperations()
	// 				if err != nil {
	// 					log.Error().Err(err).Msg("Session operation failed")
	// 					error_count++
	// 				}
	// 				time_taken = append(time_taken, request_times...)
	// 			}()
	// 		}
	// 	}
	return nil, nil
}
