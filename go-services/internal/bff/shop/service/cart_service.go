package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/shop/model"
	"github.com/athena-lms/go-services/internal/bff/shop/repository"
	"github.com/athena-lms/go-services/internal/common/errors"
)

const deliveryFee = 200.00

type CartService struct {
	cartRepo    *repository.CartRepo
	productRepo *repository.ProductRepo
}

func NewCartService(cartRepo *repository.CartRepo, productRepo *repository.ProductRepo) *CartService {
	return &CartService{
		cartRepo:    cartRepo,
		productRepo: productRepo,
	}
}

func (s *CartService) GetCart(ctx context.Context, tenantID string, userID uuid.UUID) (*model.CartResponse, error) {
	items, err := s.cartRepo.FindByUser(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	return s.buildCartResponse(ctx, items)
}

func (s *CartService) AddToCart(ctx context.Context, tenantID string, userID, productID uuid.UUID, quantity int, bnplPlanID *uuid.UUID) (*model.CartResponse, error) {
	if quantity <= 0 {
		quantity = 1
	}

	// Validate product exists and is active.
	product, err := s.productRepo.FindByID(ctx, productID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFoundResource("Product", productID.String())
		}
		return nil, err
	}
	if !product.Active {
		return nil, errors.BadRequest("product is not available")
	}
	if product.StockQuantity < quantity {
		return nil, errors.BadRequest("insufficient stock")
	}

	item := &model.CartItem{
		ID:             uuid.New(),
		TenantID:       tenantID,
		UserID:         userID,
		ProductID:      productID,
		Quantity:       quantity,
		SelectedBNPLID: bnplPlanID,
	}
	if err := s.cartRepo.Upsert(ctx, item); err != nil {
		return nil, err
	}

	return s.GetCart(ctx, tenantID, userID)
}

func (s *CartService) UpdateCartItem(ctx context.Context, tenantID string, userID, productID uuid.UUID, quantity int) (*model.CartResponse, error) {
	if quantity <= 0 {
		return s.RemoveFromCart(ctx, tenantID, userID, productID)
	}

	// Check product stock.
	product, err := s.productRepo.FindByID(ctx, productID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFoundResource("Product", productID.String())
		}
		return nil, err
	}
	if product.StockQuantity < quantity {
		return nil, errors.BadRequest("insufficient stock")
	}

	if err := s.cartRepo.UpdateQuantity(ctx, tenantID, userID, productID, quantity); err != nil {
		return nil, err
	}
	return s.GetCart(ctx, tenantID, userID)
}

func (s *CartService) RemoveFromCart(ctx context.Context, tenantID string, userID, productID uuid.UUID) (*model.CartResponse, error) {
	if err := s.cartRepo.Delete(ctx, tenantID, userID, productID); err != nil {
		return nil, err
	}
	return s.GetCart(ctx, tenantID, userID)
}

func (s *CartService) ClearCart(ctx context.Context, tenantID string, userID uuid.UUID) error {
	return s.cartRepo.ClearCart(ctx, tenantID, userID)
}

func (s *CartService) buildCartResponse(ctx context.Context, items []model.CartItem) (*model.CartResponse, error) {
	resp := &model.CartResponse{
		Items:       make([]model.CartItemResponse, 0, len(items)),
		DeliveryFee: deliveryFee,
	}

	if len(items) == 0 {
		resp.DeliveryFee = 0
		return resp, nil
	}

	var subtotal float64
	for _, item := range items {
		product, err := s.productRepo.FindByID(ctx, item.ProductID)
		if err != nil {
			slog.Warn("cart item references missing product", "productId", item.ProductID)
			continue
		}

		itemSubtotal := product.Price * float64(item.Quantity)
		var imageURL string
		var urls []string
		if err := json.Unmarshal(product.ImageURLs, &urls); err == nil && len(urls) > 0 {
			imageURL = urls[0]
		}

		resp.Items = append(resp.Items, model.CartItemResponse{
			ProductID:    product.ID,
			ProductName:  product.Name,
			ProductImage: imageURL,
			UnitPrice:    product.Price,
			Quantity:     item.Quantity,
			BNPLPlanID:   item.SelectedBNPLID,
			Subtotal:     itemSubtotal,
		})
		subtotal += itemSubtotal
	}

	resp.Subtotal = subtotal
	resp.Total = subtotal + resp.DeliveryFee
	resp.ItemCount = len(resp.Items)
	return resp, nil
}
