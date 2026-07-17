package service

import (
	"context"
	"database/sql"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/shop/model"
	"github.com/athena-lms/go-services/internal/bff/shop/repository"
	"github.com/athena-lms/go-services/internal/common/errors"
)

type FavoriteService struct {
	favoriteRepo *repository.FavoriteRepo
	productRepo  *repository.ProductRepo
}

func NewFavoriteService(favoriteRepo *repository.FavoriteRepo, productRepo *repository.ProductRepo) *FavoriteService {
	return &FavoriteService{
		favoriteRepo: favoriteRepo,
		productRepo:  productRepo,
	}
}

func (s *FavoriteService) ListFavorites(ctx context.Context, tenantID string, userID uuid.UUID) (*model.FavoritesListResponse, error) {
	favs, err := s.favoriteRepo.FindByUser(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	products := make([]model.ProductResponse, 0, len(favs))
	for _, fav := range favs {
		p, err := s.productRepo.FindByID(ctx, fav.ProductID)
		if err != nil {
			continue
		}
		products = append(products, p.ToResponse())
	}

	return &model.FavoritesListResponse{
		Products: products,
		Count:    len(products),
	}, nil
}

func (s *FavoriteService) ToggleFavorite(ctx context.Context, tenantID string, userID, productID uuid.UUID) (*model.FavoriteToggleResponse, error) {
	// Validate product exists.
	_, err := s.productRepo.FindByID(ctx, productID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFoundResource("Product", productID.String())
		}
		return nil, err
	}

	exists, err := s.favoriteRepo.Exists(ctx, tenantID, userID, productID)
	if err != nil {
		return nil, err
	}

	if exists {
		if err := s.favoriteRepo.Delete(ctx, tenantID, userID, productID); err != nil {
			return nil, err
		}
		return &model.FavoriteToggleResponse{
			ProductID:  productID,
			IsFavorite: false,
			Message:    "Removed from favorites",
		}, nil
	}

	fav := &model.Favorite{
		ID:        uuid.New(),
		TenantID:  tenantID,
		UserID:    userID,
		ProductID: productID,
	}
	if err := s.favoriteRepo.Create(ctx, fav); err != nil {
		return nil, err
	}
	return &model.FavoriteToggleResponse{
		ProductID:  productID,
		IsFavorite: true,
		Message:    "Added to favorites",
	}, nil
}
