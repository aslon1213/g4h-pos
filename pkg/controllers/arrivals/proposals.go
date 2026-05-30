package arrivals

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel"
)

var branches = []string{"xonobod", "polevoy"}

type ProposalsHandlers struct {
	ctx                 context.Context
	ProposalsCollection *mongo.Collection
}

func New(db *mongo.Database) *ProposalsHandlers {
	return &ProposalsHandlers{
		ctx:                 context.Background(),
		ProposalsCollection: db.Collection("proposals"),
	}
}

// GetImage godoc
// @Summary Get image by name
// @Description Retrieves an image file by its name
// @Security BearerAuth
// @Tags proposals
// @Produce octet-stream
// @Param image_name query string true "Image name"
// @Success 200 {file} binary "Image file"
// @Failure 404 {object} models.ErrorOutput "Image not found"
// @Router /api/v1/admin/proposals/images [get]
func (h *ProposalsHandlers) GetImage(c *fiber.Ctx) error {
	tracer := otel.Tracer("proposals-handlers")
	_, span := tracer.Start(h.ctx, "GetImage")
	defer span.End()

	imageName := c.Query("image_name", c.Params("image_name"))
	imagePath := filepath.Join("images", imageName)

	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		log.Warn().Str("image_name", imageName).Msg("get_image.not_found")
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Image not found",
		})
	}

	return c.SendFile(imagePath)
}

// GetImageByProposalID godoc
// @Summary Get image upload page by proposal ID
// @Description Retrieves the image upload page for a specific proposal record
// @Security BearerAuth
// @Tags proposals
// @Produce html
// @Param proposal_id query string true "Proposal ID"
// @Success 200 {string} string "HTML page"
// @Failure 400 {object} models.ErrorOutput "Invalid proposal ID"
// @Failure 404 {object} models.ErrorOutput "Proposal not found"
// @Router /api/v1/admin/proposals/image [get]
func (h *ProposalsHandlers) GetImageByProposalID(c *fiber.Ctx) error {
	tracer := otel.Tracer("proposals-handlers")
	ctx, span := tracer.Start(h.ctx, "GetImageByProposalID")
	defer span.End()

	proposalID := c.Query("proposal_id", c.Params("proposal_id"))
	objectID, err := bson.ObjectIDFromHex(proposalID)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("get_image_by_proposal_id.invalid_id")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid proposal ID",
		})
	}

	var proposal models.ProductProposal
	err = h.ProposalsCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&proposal)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("get_image_by_proposal_id.not_found")
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Proposal not found",
		})
	}

	return c.Render("image_upload", fiber.Map{
		"proposal_id": proposalID,
		"image":       proposal.ImageFile,
		"url":         fmt.Sprintf("/proposals/%s/image", proposalID),
	})
}

// UploadImage godoc
// @Summary Upload image for proposal
// @Description Uploads an image file for a specific proposal record
// @Security BearerAuth
// @Tags proposals
// @Accept multipart/form-data
// @Produce json
// @Param proposal_id query string true "Proposal ID"
// @Param file formData file true "Image file to upload"
// @Success 303 {string} string "Redirect to proposals list"
// @Failure 400 {object} models.ErrorOutput "Invalid proposal ID or no file uploaded"
// @Failure 404 {object} models.ErrorOutput "Proposal not found"
// @Failure 500 {object} models.ErrorOutput "Failed to save file or update proposal"
// @Router /api/v1/admin/proposals/image [post]
func (h *ProposalsHandlers) UploadImage(c *fiber.Ctx) error {
	tracer := otel.Tracer("proposals-handlers")
	ctx, span := tracer.Start(h.ctx, "UploadImage")
	defer span.End()

	proposalID := c.Query("proposal_id", c.Params("proposal_id"))
	objectID, err := bson.ObjectIDFromHex(proposalID)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("upload_image.invalid_id")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid proposal ID",
		})
	}

	// Get the uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("upload_image.no_file")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "No file uploaded",
		})
	}

	// Check if proposal exists
	var proposal models.ProductProposal
	err = h.ProposalsCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&proposal)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("upload_image.proposal_not_found")
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Proposal not found",
		})
	}

	// Delete old file if exists
	if proposal.ImageFile != nil && *proposal.ImageFile != "" {
		oldFilePath := filepath.Join(".", *proposal.ImageFile)
		if _, err := os.Stat(oldFilePath); err == nil {
			if err := os.Remove(oldFilePath); err != nil {
				log.Warn().Err(err).Str("proposal_id", proposalID).Msg("upload_image.delete_old_file_failed")
			}
		}
	}

	// Save new file
	newFileName := fmt.Sprintf("%s_%s", proposalID, file.Filename)
	newFilePath := filepath.Join("images", newFileName)

	if err := c.SaveFile(file, newFilePath); err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("upload_image.save_failed")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save file",
		})
	}

	// Update proposal with new image path
	_, err = h.ProposalsCollection.UpdateOne(
		ctx,
		bson.M{"_id": objectID},
		bson.M{"$set": bson.M{"image_file": newFilePath}},
	)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("upload_image.update_failed")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update proposal",
		})
	}

	log.Info().Str("proposal_id", proposalID).Str("filename", file.Filename).Msg("upload_image.success")
	return c.Redirect("/proposals/", http.StatusSeeOther)
}

// NewProposals godoc
// @Summary Create new proposals
// @Description Creates new proposal records for a specific branch
// @Security BearerAuth
// @Tags proposals
// @Accept json
// @Produce json
// @Param branch query string true "Branch name"
// @Param body body []string true "List of proposal names"
// @Success 200 {object} models.MessageResponse "Proposals created successfully"
// @Failure 400 {object} models.ErrorOutput "Invalid branch or request body"
// @Failure 500 {object} models.ErrorOutput "Failed to create proposals"
// @Router /api/v1/admin/proposals/new [post]
func (h *ProposalsHandlers) NewProposals(c *fiber.Ctx) error {
	tracer := otel.Tracer("proposals-handlers")
	ctx, span := tracer.Start(h.ctx, "NewProposals")
	defer span.End()

	branch := c.Query("branch", c.Params("branch"))
	if !h.checkBranch(branch) {
		log.Error().Str("branch", branch).Msg("new_proposals.invalid_branch")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid branch",
		})
	}

	var proposalNames []string
	if err := c.BodyParser(&proposalNames); err != nil {
		log.Error().Err(err).Str("branch", branch).Msg("new_proposals.parse_error")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	var proposalsToInsert []interface{}
	now := time.Now()

	for _, name := range proposalNames {
		proposal := models.ProductProposal{
			ID:        bson.NewObjectID(),
			Name:      name,
			Date:      now,
			Branch:    branch,
			Fulfilled: false,
		}
		proposalsToInsert = append(proposalsToInsert, proposal)
	}

	result, err := h.ProposalsCollection.InsertMany(ctx, proposalsToInsert)
	if err != nil {
		log.Error().Err(err).Str("branch", branch).Msg("new_proposals.insert_failed")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proposals",
		})
	}

	var outputIDs []string
	for _, id := range result.InsertedIDs {
		outputIDs = append(outputIDs, id.(bson.ObjectID).Hex())
	}

	log.Info().Str("branch", branch).Int("count", len(proposalNames)).Msg("new_proposals.success")
	return c.JSON(fiber.Map{
		"message":      "Proposals created successfully",
		"proposal_ids": outputIDs,
	})
}

// GetProposals godoc
// @Summary Get all proposals
// @Description Retrieves all proposals with optional filters
// @Security BearerAuth
// @Tags proposals
// @Produce json
// @Param name query string false "Filter by name (case-insensitive)"
// @Param branch query string false "Filter by branch (case-insensitive)"
// @Param fulfilled query string false "Filter by fulfilled status (true/false)" default(false)
// @Param date_from query string false "Filter by start date (YYYY-MM-DD)"
// @Param date_to query string false "Filter by end date (YYYY-MM-DD)"
// @Success 200 {array} models.ProductProposal "List of proposals"
// @Failure 400 {object} models.ErrorOutput "Invalid date format"
// @Failure 500 {object} models.ErrorOutput "Failed to fetch or decode proposals"
// @Router /api/v1/admin/proposals [get]
func (h *ProposalsHandlers) GetProposals(c *fiber.Ctx) error {
	tracer := otel.Tracer("proposals-handlers")
	ctx, span := tracer.Start(h.ctx, "GetProposals")
	defer span.End()

	filter := bson.M{}

	// Name filter
	if name := c.Query("name"); name != "" {
		filter["name"] = bson.M{"$regex": regexp.QuoteMeta(name), "$options": "i"}
	}

	// Branch filter
	if branch := c.Query("branch"); branch != "" {
		filter["branch"] = bson.M{"$regex": regexp.QuoteMeta(branch), "$options": "i"}
	}

	// Fulfilled filter
	if fulfilled := c.Query("fulfilled", "false"); fulfilled != "" {
		if fulfilled == "true" {
			filter["fulfilled"] = true
		} else if fulfilled == "false" {
			filter["fulfilled"] = false
		}
	}

	// Date filters
	if dateFrom := c.Query("date_from"); dateFrom != "" {
		parsedDate, err := time.Parse("2006-01-02", dateFrom)
		if err != nil {
			log.Error().Str("date_from", dateFrom).Msg("get_proposals.invalid_date_from")
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid date_from format. Use YYYY-MM-DD.",
			})
		}
		filter["date"] = bson.M{"$gte": parsedDate}
	} else {
		filter["date"] = bson.M{"$gte": time.Now().Add(-30 * 24 * time.Hour)}
	}

	if dateTo := c.Query("date_to"); dateTo != "" {
		parsedDate, err := time.Parse("2006-01-02", dateTo)
		if err != nil {
			log.Error().Str("date_to", dateTo).Msg("get_proposals.invalid_date_to")
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid date_to format. Use YYYY-MM-DD.",
			})
		}
		parsedDate = parsedDate.Add(24 * time.Hour) // Add one day
		if _, exists := filter["date"]; exists {
			filter["date"].(bson.M)["$lte"] = parsedDate
		} else {
			filter["date"] = bson.M{"$lte": parsedDate}
		}
	} else {

		if _, exists := filter["date"]; exists {
			filter["date"].(bson.M)["$lte"] = time.Now()
		} else {
			filter["date"] = bson.M{"$lte": time.Now()}
		}

	}

	cursor, err := h.ProposalsCollection.Find(ctx, filter)
	if err != nil {
		log.Error().Err(err).Msg("get_proposals.find_failed")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch proposals",
		})
	}
	defer cursor.Close(ctx)

	var proposals []models.ProductProposal
	if err := cursor.All(ctx, &proposals); err != nil {
		log.Error().Err(err).Msg("get_proposals.decode_failed")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to decode proposals",
		})
	}

	// Format dates for response
	response := []fiber.Map{}
	for _, proposal := range proposals {
		response = append(response, fiber.Map{
			"_id":       proposal.ID.Hex(),
			"name":      proposal.Name,
			"date":      proposal.Date.Format("02-01-2006"),
			"branch":    proposal.Branch,
			"fulfilled": proposal.Fulfilled,
		})
	}

	log.Info().Interface("filter", filter).Int("count", len(proposals)).Msg("get_proposals.success")
	return c.JSON(response)
}

// GetProposalDetail godoc
// @Summary Get proposal detail
// @Description Retrieves detailed information for a specific proposal record
// @Security BearerAuth
// @Tags proposals
// @Produce html
// @Param proposal_id query string true "Proposal ID"
// @Success 200 {string} string "HTML page with proposal details"
// @Failure 400 {object} models.ErrorOutput "Invalid proposal ID"
// @Failure 404 {object} models.ErrorOutput "Proposal not found"
// @Router /api/v1/admin/proposals/detail [get]
func (h *ProposalsHandlers) GetProposalDetail(c *fiber.Ctx) error {
	tracer := otel.Tracer("proposals-handlers")
	ctx, span := tracer.Start(h.ctx, "GetProposalDetail")
	defer span.End()

	proposalID := c.Query("proposal_id", c.Params("proposal_id"))
	objectID, err := bson.ObjectIDFromHex(proposalID)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("get_proposal_detail.invalid_id")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid proposal ID",
		})
	}

	var proposal models.ProductProposal
	err = h.ProposalsCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&proposal)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("get_proposal_detail.not_found")
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Proposal not found",
		})
	}

	log.Info().Str("proposal_id", proposalID).Msg("get_proposal_detail.success")
	return c.Render("proposal", fiber.Map{
		"proposal": fiber.Map{
			"_id":       proposal.ID.Hex(),
			"name":      proposal.Name,
			"date":      proposal.Date.Format("02-01-2006"),
			"branch":    proposal.Branch,
			"fulfilled": proposal.Fulfilled,
		},
		"action": "view",
	})
}

// EditProposalRequest represents the request body for editing a proposal
type EditProposalRequest struct {
	Name      string `json:"name" form:"name"`
	Branch    string `json:"branch" form:"branch"`
	Fulfilled *bool  `json:"fulfilled" form:"fulfilled"`
}

// EditProposal godoc
// @Summary Edit proposal
// @Description Updates an existing proposal record
// @Security BearerAuth
// @Tags proposals
// @Accept json
// @Produce json
// @Param proposal_id query string true "Proposal ID"
// @Param body body EditProposalRequest false "Proposal update data"
// @Success 302 {string} string "Redirect to proposals list"
// @Failure 400 {object} models.ErrorOutput "Invalid proposal ID or request data"
// @Failure 500 {object} models.ErrorOutput "Failed to update proposal"
// @Router /api/v1/admin/proposals/edit [put]
func (h *ProposalsHandlers) EditProposal(c *fiber.Ctx) error {
	tracer := otel.Tracer("proposals-handlers")
	ctx, span := tracer.Start(h.ctx, "EditProposal")
	defer span.End()

	proposalID := c.Query("proposal_id", c.Params("proposal_id"))
	objectID, err := bson.ObjectIDFromHex(proposalID)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("edit_proposal.invalid_id")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid proposal ID",
		})
	}

	var req EditProposalRequest
	if err := c.BodyParser(&req); err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("edit_proposal.parse_error")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request data",
		})
	}

	update := bson.M{"$set": bson.M{}}

	if req.Name != "" {
		update["$set"].(bson.M)["name"] = req.Name
	}
	if req.Branch != "" {
		update["$set"].(bson.M)["branch"] = req.Branch
	}
	if req.Fulfilled != nil {
		update["$set"].(bson.M)["fulfilled"] = *req.Fulfilled
	}

	_, err = h.ProposalsCollection.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("edit_proposal.update_failed")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update proposal",
		})
	}

	log.Info().Str("proposal_id", proposalID).Interface("updates", update["$set"]).Msg("edit_proposal.success")
	return c.Redirect("/proposals", http.StatusFound)
}

// DeleteProposal godoc
// @Summary Delete proposal
// @Description Deletes a proposal record and its associated image file
// @Security BearerAuth
// @Tags proposals
// @Produce json
// @Param proposal_id query string true "Proposal ID"
// @Success 302 {string} string "Redirect to proposals list"
// @Failure 400 {object} models.ErrorOutput "Invalid proposal ID"
// @Failure 404 {object} models.ErrorOutput "Proposal not found"
// @Failure 500 {object} models.ErrorOutput "Failed to delete proposal"
// @Router /api/v1/admin/proposals/delete [delete]
func (h *ProposalsHandlers) DeleteProposal(c *fiber.Ctx) error {
	tracer := otel.Tracer("proposals-handlers")
	ctx, span := tracer.Start(h.ctx, "DeleteProposal")
	defer span.End()

	proposalID := c.Query("proposal_id", c.Params("proposal_id"))
	objectID, err := bson.ObjectIDFromHex(proposalID)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("delete_proposal.invalid_id")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid proposal ID",
		})
	}

	// Get the proposal first to check for image file
	var proposal models.ProductProposal
	err = h.ProposalsCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&proposal)
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("delete_proposal.not_found")
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Proposal not found",
		})
	}

	// Delete the proposal
	_, err = h.ProposalsCollection.DeleteOne(ctx, bson.M{"_id": objectID})
	if err != nil {
		log.Error().Err(err).Str("proposal_id", proposalID).Msg("delete_proposal.delete_failed")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete proposal",
		})
	}

	// Delete the image file if it exists
	if proposal.ImageFile != nil && *proposal.ImageFile != "" {
		imageFilePath := filepath.Join(".", *proposal.ImageFile)
		if _, err := os.Stat(imageFilePath); err == nil {
			if err := os.Remove(imageFilePath); err != nil {
				log.Warn().Err(err).Str("proposal_id", proposalID).Msg("delete_proposal.image_delete_failed")
			}
		}
	}

	log.Info().Str("proposal_id", proposalID).Msg("delete_proposal.success")
	return c.Redirect("/proposals", http.StatusFound)
}

// FulfillProposals godoc
// @Summary Fulfill proposals
// @Description Marks multiple proposals as fulfilled for a specific branch
// @Security BearerAuth
// @Tags proposals
// @Produce json
// @Param branch query string true "Branch name"
// @Param ids query string true "Comma-separated list of proposal IDs"
// @Success 200 {object} models.MessageResponse "Fulfillment result"
// @Failure 400 {object} models.ErrorOutput "Invalid branch or no valid IDs provided"
// @Failure 500 {object} models.ErrorOutput "Failed to fulfill proposals"
// @Router /api/v1/admin/proposals/fulfill [get]
func (h *ProposalsHandlers) FulfillProposals(c *fiber.Ctx) error {
	tracer := otel.Tracer("proposals-handlers")
	ctx, span := tracer.Start(h.ctx, "FulfillProposals")
	defer span.End()

	branch := c.Query("branch", c.Params("branch"))
	if !h.checkBranch(branch) {
		log.Error().Str("branch", branch).Msg("fulfill_proposals.invalid_branch")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid branch",
		})
	}

	idsParam := c.Query("ids", "")
	if idsParam == "" {
		log.Error().Str("branch", branch).Msg("fulfill_proposals.no_ids")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "No proposal IDs provided",
		})
	}

	proposalIDs := strings.Split(idsParam, ",")
	var objectIDs []bson.ObjectID

	for _, id := range proposalIDs {
		objectID, err := bson.ObjectIDFromHex(strings.TrimSpace(id))
		if err != nil {
			log.Error().Err(err).Str("id", id).Msg("fulfill_proposals.invalid_id")
			continue
		}
		objectIDs = append(objectIDs, objectID)
	}

	if len(objectIDs) == 0 {
		log.Error().Str("branch", branch).Msg("fulfill_proposals.no_valid_ids")
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "No valid proposal IDs provided",
		})
	}

	result, err := h.ProposalsCollection.UpdateMany(
		ctx,
		bson.M{
			"_id":    bson.M{"$in": objectIDs},
			"branch": branch,
		},
		bson.M{"$set": bson.M{"fulfilled": true}},
	)
	if err != nil {
		log.Error().Err(err).Str("branch", branch).Msg("fulfill_proposals.update_failed")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fulfill proposals",
		})
	}

	log.Info().
		Str("branch", branch).
		Strs("proposal_ids", proposalIDs).
		Int64("matched_count", result.MatchedCount).
		Int64("modified_count", result.ModifiedCount).
		Msg("fulfill_proposals.success")

	return c.JSON(fiber.Map{
		"acknowledged":        true,
		"total_number_of_ids": len(proposalIDs),
		"matched_count":       result.MatchedCount,
		"modified_count":      result.ModifiedCount,
		"upserted_id":         nil,
	})
}

// GeneratePDF godoc
// @Summary Generate PDF
// @Description Generates a PDF document with unfulfilled proposals
// @Security BearerAuth
// @Tags proposals
// @Produce json
// @Success 200 {object} models.MessageResponse "PDF generation result with proposals data"
// @Failure 500 {object} models.ErrorOutput "Failed to fetch or decode proposals"
// @Router /api/v1/admin/proposals/pdf/pdf [get]
func (h *ProposalsHandlers) GeneratePDF(c *fiber.Ctx) error {
	tracer := otel.Tracer("proposals-handlers")
	ctx, span := tracer.Start(h.ctx, "GeneratePDF")
	defer span.End()

	filter := bson.M{"fulfilled": false}
	cursor, err := h.ProposalsCollection.Find(ctx, filter)
	if err != nil {
		log.Error().Err(err).Msg("generate_pdf.find_failed")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch proposals",
		})
	}
	defer cursor.Close(ctx)

	var proposals []models.ProductProposal
	if err := cursor.All(ctx, &proposals); err != nil {
		log.Error().Err(err).Msg("generate_pdf.decode_failed")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to decode proposals",
		})
	}

	// Format proposals for PDF
	var formattedProposals []fiber.Map
	baseURL := os.Getenv("PROPOSALS_BASE_URL")
	for _, proposal := range proposals {
		imageFile := ""
		if proposal.ImageFile != nil && *proposal.ImageFile != "" && baseURL != "" {
			imageFile = baseURL + *proposal.ImageFile
		}

		formattedProposals = append(formattedProposals, fiber.Map{
			"_id":        proposal.ID.Hex(),
			"name":       proposal.Name,
			"date":       proposal.Date.Format("02-01-2006"),
			"branch":     proposal.Branch,
			"fulfilled":  proposal.Fulfilled,
			"image_file": imageFile,
		})
	}

	// Render PDF template (this would need to be implemented with a PDF library)
	// For now, returning JSON response
	log.Info().Int("proposals_count", len(proposals)).Msg("generate_pdf.success")

	return c.JSON(fiber.Map{
		"message":   "PDF generation not implemented yet",
		"proposals": formattedProposals,
	})
}

// Helper function to check branch validity
func (h *ProposalsHandlers) checkBranch(branch string) bool {

	// Implement branch validation logic here
	// This is a placeholder implementation
	validBranches := branches
	for _, validBranch := range validBranches {
		if branch == validBranch {
			return true
		}
	}
	return false
}
