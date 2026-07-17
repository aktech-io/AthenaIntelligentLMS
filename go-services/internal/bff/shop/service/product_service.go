package service

import (
	"context"
	"database/sql"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/shop/model"
	"github.com/athena-lms/go-services/internal/bff/shop/repository"
	"github.com/athena-lms/go-services/internal/common/errors"
)

type ProductService struct {
	categoryRepo *repository.CategoryRepo
	productRepo  *repository.ProductRepo
}

func NewProductService(categoryRepo *repository.CategoryRepo, productRepo *repository.ProductRepo) *ProductService {
	return &ProductService{
		categoryRepo: categoryRepo,
		productRepo:  productRepo,
	}
}

func (s *ProductService) ListCategories(ctx context.Context, tenantID string) ([]model.ShopCategoryResponse, error) {
	cats, err := s.categoryRepo.FindAllActive(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	resp := make([]model.ShopCategoryResponse, len(cats))
	for i := range cats {
		resp[i] = cats[i].ToResponse()
	}
	return resp, nil
}

func (s *ProductService) ListFeatured(ctx context.Context, tenantID string) ([]model.ProductResponse, error) {
	products, err := s.productRepo.FindFeatured(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	resp := make([]model.ProductResponse, len(products))
	for i := range products {
		resp[i] = products[i].ToResponse()
	}
	return resp, nil
}

func (s *ProductService) SearchProducts(ctx context.Context, tenantID string, categoryID *uuid.UUID, query, sort string, page, size int) ([]model.ProductResponse, int64, error) {
	products, total, err := s.productRepo.Search(ctx, tenantID, categoryID, query, sort, page, size)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]model.ProductResponse, len(products))
	for i := range products {
		resp[i] = products[i].ToResponse()
	}
	return resp, total, nil
}

func (s *ProductService) GetProduct(ctx context.Context, id uuid.UUID) (*model.ProductResponse, error) {
	p, err := s.productRepo.FindByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFoundResource("Product", id.String())
		}
		return nil, err
	}
	resp := p.ToResponse()
	return &resp, nil
}
