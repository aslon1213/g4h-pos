package products

import (
	"fmt"
	"path"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// UploadProductImage godoc
// @Security BearerAuth
// @Summary Upload a product image
// @Description Uploads an image file for a product and stores it in S3
// @Tags products
// @Accept multipart/form-data
// @Produce json
// @Param product_id path string true "Product ID"
// @Param image formData file true "Image file to upload"
// @Success 200 {object} map[string]string
// @Failure 400 {string} string
// @Failure 500 {string} string
// @Router /api/v1/admin/products/{product_id}/images [post]
func (p *ProductsController) UploadProductImage(c *fiber.Ctx) error {
	file, err := c.FormFile("image")
	if err != nil {
		log.Error().Err(err).Msg("Failed to get image file")
		return err
	}

	productID := c.Params("product_id")
	if productID == "" {
		return fmt.Errorf("product_id is required")
	}

	// Generate unique filename
	filename := uuid.New().String() + path.Ext(file.Filename)
	key := fmt.Sprintf("products/%s/%s", productID, filename)

	// Open file
	fileContent, err := file.Open()
	if err != nil {
		log.Error().Err(err).Msg("Failed to open file")
		return err
	}
	defer fileContent.Close()

	// Upload to S3
	err = p.S3Client.UploadFile(file, key)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upload to S3")
		return err
	}

	// Update product in database with new image key
	update := bson.M{
		"$push": bson.M{
			"images": key,
		},
	}
	_, err = p.ProductsCollection.UpdateOne(c.Context(), bson.M{"_id": productID}, update)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update product with image")
		return err
	}

	return c.JSON(fiber.Map{
		"image_key": key,
	})
}

// DeleteProductImage godoc
// @Security BearerAuth
// @Summary Delete a product image
// @Description Deletes a product image from S3 and removes reference from database
// @Tags products
// @Produce json
// @Param product_id path string true "Product ID"
// @Param key path string true "Image key to delete"
// @Success 200 {string} string
// @Failure 400 {string} string
// @Failure 500 {string} string
// @Router /api/v1/admin/products/{product_id}/images/{key} [delete]
func (p *ProductsController) DeleteProductImage(c *fiber.Ctx) error {
	key := c.Params("key")
	if key == "" {
		return fmt.Errorf("image key is required")
	}

	productID := c.Params("product_id")
	if productID == "" {
		return fmt.Errorf("product_id is required")
	}

	// Delete from S3
	err := p.S3Client.DeleteFile(key)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete from S3")
		return err
	}

	// Remove image key from product in database
	update := bson.M{
		"$pull": bson.M{
			"images": key,
		},
	}
	_, err = p.ProductsCollection.UpdateOne(c.Context(), bson.M{"_id": productID}, update)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update product after image deletion")
		return err
	}

	return c.SendStatus(fiber.StatusOK)
}

// GetImagesOfProduct godoc
// @Security BearerAuth
// @Summary Get all images of a product
// @Description Returns a list of image URLs for a given product
// @Tags products
// @Produce json
// @Param product_id path string true "Product ID"
// @Success 200 {object} map[string][]string
// @Failure 400 {string} string
// @Failure 500 {string} string
// @Router /api/v1/admin/products/{product_id}/images [get]
func (p *ProductsController) GetImagesOfProduct(c *fiber.Ctx) error {
	productID := c.Params("product_id")
	if productID == "" {
		return fmt.Errorf("product_id is required")
	}

	prefix := fmt.Sprintf("products/%s/", productID)

	// List objects with product prefix
	images, err := p.S3Client.ListFiles(prefix)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list objects from S3")
		return err
	}

	return c.JSON(fiber.Map{
		"images": images,
	})
}

// GetImage godoc
// @Security BearerAuth
// @Summary Get a single product image
// @Description Returns the image file for a given image key
// @Tags products
// @Produce octet-stream
// @Param key path string true "Image key"
// @Success 200 {file} binary
// @Failure 400 {string} string
// @Failure 500 {string} string
// @Router /api/v1/admin/products/images/{key} [get]
func (p *ProductsController) GetImage(c *fiber.Ctx) error {
	key := c.Params("key")
	if key == "" {
		return fmt.Errorf("image key is required")
	}

	// Get presigned URL for object
	s3_object, err := p.S3Client.GetFile(key)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate presigned URL")
		return err
	}

	return c.SendStream(s3_object.Body, int(*s3_object.ContentLength))
}
