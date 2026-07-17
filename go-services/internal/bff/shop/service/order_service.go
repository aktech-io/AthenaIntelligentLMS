package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/shop/client"
	"github.com/athena-lms/go-services/internal/bff/shop/model"
	"github.com/athena-lms/go-services/internal/bff/shop/publisher"
	"github.com/athena-lms/go-services/internal/bff/shop/repository"
	"github.com/athena-lms/go-services/internal/common/auth"
	"github.com/athena-lms/go-services/internal/common/errors"
)

type OrderService struct {
	orderRepo     *repository.OrderRepo
	cartRepo      *repository.CartRepo
	productRepo   *repository.ProductRepo
	bnplRepo      *repository.BNPLRepo
	accountClient *client.AccountClient
	paymentClient *client.PaymentClient
	loanClient    *client.LoanOriginationClient
	publisher     *publisher.EventPublisher
}

func NewOrderService(
	orderRepo *repository.OrderRepo,
	cartRepo *repository.CartRepo,
	productRepo *repository.ProductRepo,
	bnplRepo *repository.BNPLRepo,
	accountClient *client.AccountClient,
	paymentClient *client.PaymentClient,
	loanClient *client.LoanOriginationClient,
	pub *publisher.EventPublisher,
) *OrderService {
	return &OrderService{
		orderRepo:     orderRepo,
		cartRepo:      cartRepo,
		productRepo:   productRepo,
		bnplRepo:      bnplRepo,
		accountClient: accountClient,
		paymentClient: paymentClient,
		loanClient:    loanClient,
		publisher:     pub,
	}
}

type PlaceOrderRequest struct {
	PaymentType          model.PaymentType      `json:"paymentType"`
	DeliveryAddress      map[string]interface{} `json:"deliveryAddress"`
	BNPLPlanID           *uuid.UUID             `json:"bnplPlanId"`
	DepositPaymentMethod *string                `json:"depositPaymentMethod"`
	Notes                *string                `json:"notes"`
}

func (s *OrderService) PlaceOrder(ctx context.Context, tenantID string, userID uuid.UUID, req PlaceOrderRequest) (*model.OrderResponse, error) {
	customerID := auth.CustomerIDStrFromContext(ctx)

	// 1. Validate cart not empty.
	cartItems, err := s.cartRepo.FindByUser(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	if len(cartItems) == 0 {
		return nil, errors.BadRequest("cart is empty")
	}

	// 2. If BNPL, validate plan exists.
	var bnplPlan *model.BNPLPlan
	if req.PaymentType == model.PaymentBNPL {
		if req.BNPLPlanID == nil {
			return nil, errors.BadRequest("BNPL plan is required for BNPL payment")
		}
		plan, err := s.bnplRepo.FindByID(ctx, *req.BNPLPlanID)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, errors.NotFoundResource("BNPL Plan", req.BNPLPlanID.String())
			}
			return nil, err
		}
		if customerID == "" {
			return nil, errors.BadRequest("customer identity required for BNPL")
		}
		bnplPlan = plan
	}

	// Start transaction.
	tx, err := s.orderRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 3. Validate products, check stock, decrement stock, build order items.
	var subtotal float64
	orderItems := make([]model.OrderItem, 0, len(cartItems))
	for _, ci := range cartItems {
		product, err := s.productRepo.FindByID(ctx, ci.ProductID)
		if err != nil {
			return nil, errors.BadRequest(fmt.Sprintf("product %s not found", ci.ProductID))
		}
		if !product.Active {
			return nil, errors.BadRequest(fmt.Sprintf("product %s is not available", product.Name))
		}

		if err := s.productRepo.DecrementStock(ctx, tx, product.ID, ci.Quantity); err != nil {
			return nil, errors.BadRequest(fmt.Sprintf("insufficient stock for %s", product.Name))
		}

		var imageURL string
		var urls []string
		if err := json.Unmarshal(product.ImageURLs, &urls); err == nil && len(urls) > 0 {
			imageURL = urls[0]
		}

		itemTotal := product.Price * float64(ci.Quantity)
		subtotal += itemTotal

		orderItems = append(orderItems, model.OrderItem{
			ID:              uuid.New(),
			ProductID:       product.ID,
			ProductName:     product.Name,
			ProductImageURL: imageURL,
			Quantity:        ci.Quantity,
			UnitPrice:       product.Price,
			TotalPrice:      itemTotal,
		})
	}

	// 4. Calculate totals.
	totalAmount := subtotal + deliveryFee

	// 5. BNPL: validate amount range.
	if bnplPlan != nil {
		if totalAmount < bnplPlan.MinAmount || totalAmount > bnplPlan.MaxAmount {
			return nil, errors.BadRequest(fmt.Sprintf("order total %.2f is outside BNPL plan range [%.2f - %.2f]", totalAmount, bnplPlan.MinAmount, bnplPlan.MaxAmount))
		}
	}

	// 6. Generate order number.
	orderNumber := fmt.Sprintf("ATH-%d-%s", time.Now().UnixMilli(), uuid.New().String()[:4])

	// 7. Create order.
	deliveryAddrJSON, _ := json.Marshal(req.DeliveryAddress)
	order := &model.Order{
		ID:              uuid.New(),
		TenantID:        tenantID,
		UserID:          userID,
		OrderNumber:     orderNumber,
		PaymentType:     req.PaymentType,
		Status:          model.StatusPending,
		Subtotal:        subtotal,
		DeliveryFee:     deliveryFee,
		TotalAmount:     totalAmount,
		DeliveryAddress: deliveryAddrJSON,
		DepositStatus:   model.DepositNone,
		BNPLPlanID:      req.BNPLPlanID,
		Notes:           req.Notes,
	}

	if err := s.orderRepo.CreateOrder(ctx, tx, order); err != nil {
		return nil, err
	}

	// Create order items.
	for i := range orderItems {
		orderItems[i].OrderID = order.ID
		if err := s.orderRepo.CreateOrderItem(ctx, tx, &orderItems[i]); err != nil {
			return nil, err
		}
	}

	// Create ORDER_PLACED delivery event.
	desc := "Order has been placed"
	deliveryEvt := &model.DeliveryEvent{
		ID:          uuid.New(),
		OrderID:     order.ID,
		EventType:   model.EventOrderPlaced,
		Description: &desc,
	}
	if err := s.orderRepo.CreateDeliveryEvent(ctx, tx, deliveryEvt); err != nil {
		return nil, err
	}

	// Clear cart.
	if err := s.cartRepo.ClearCartTx(ctx, tx, tenantID, userID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// 8. Post-commit: BNPL flow or payment initiation.
	if req.PaymentType == model.PaymentBNPL && bnplPlan != nil {
		s.handleBNPLFlow(ctx, order, bnplPlan, customerID, req.DepositPaymentMethod)
	} else if req.PaymentType != model.PaymentCash {
		s.handlePaymentFlow(ctx, order, customerID)
	}

	// Publish event.
	s.publisher.PublishOrderPlaced(tenantID, order.ID, order.OrderNumber, userID, string(req.PaymentType), totalAmount)

	// Reload order for response.
	return s.GetOrder(ctx, order.ID)
}

func (s *OrderService) handleBNPLFlow(ctx context.Context, order *model.Order, plan *model.BNPLPlan, customerID string, depositPaymentMethod *string) {
	deposit := order.TotalAmount * plan.DepositPercentage / 100
	amountFinanced := order.TotalAmount - deposit
	order.DepositAmount = deposit
	order.AmountFinanced = &amountFinanced

	depositCollected := false
	var depositTxRef string

	// Collect deposit if > 0 and payment method is WALLET.
	if deposit > 0 && depositPaymentMethod != nil && *depositPaymentMethod == "WALLET" {
		_ = s.orderRepo.UpdateDepositStatus(ctx, order.ID, model.DepositPending, nil)

		accountID, err := s.accountClient.ResolveAccountID(ctx, customerID)
		if err != nil {
			slog.Error("failed to resolve account for deposit", "error", err, "orderId", order.ID)
			_ = s.orderRepo.UpdateDepositStatus(ctx, order.ID, model.DepositFailed, nil)
			return
		}

		debitResp, err := s.accountClient.DebitAccount(ctx, client.DebitRequest{
			AccountID:   accountID,
			Amount:      deposit,
			Description: fmt.Sprintf("BNPL deposit for order %s", order.OrderNumber),
			Reference:   order.OrderNumber + "-DEP",
		})
		if err != nil {
			slog.Error("deposit debit failed", "error", err, "orderId", order.ID)
			_ = s.orderRepo.UpdateDepositStatus(ctx, order.ID, model.DepositFailed, nil)
			return
		}

		depositTxRef = debitResp.TransactionRef
		_ = s.orderRepo.UpdateDepositStatus(ctx, order.ID, model.DepositCollected, &depositTxRef)
		depositCollected = true
	}

	// Create loan application for the financed amount.
	loanResp, err := s.loanClient.CreateAndSubmitLoanApplication(ctx, customerID, amountFinanced, order.OrderNumber)
	if err != nil {
		slog.Error("BNPL loan application failed", "error", err, "orderId", order.ID)
		// Refund deposit if collected.
		if depositCollected {
			s.refundDeposit(ctx, customerID, deposit, order.OrderNumber)
		}
		return
	}

	_ = s.orderRepo.UpdateLoanApplicationID(ctx, order.ID, loanResp.ID)
	_ = s.orderRepo.UpdateStatus(ctx, order.ID, model.StatusConfirmed)

	s.publisher.PublishBNPLApproved(order.TenantID, order.ID, order.OrderNumber, loanResp.ID)
}

func (s *OrderService) refundDeposit(ctx context.Context, customerID string, amount float64, orderNumber string) {
	accountID, err := s.accountClient.ResolveAccountID(ctx, customerID)
	if err != nil {
		slog.Error("failed to resolve account for refund", "error", err)
		return
	}
	if err := s.accountClient.CreditAccount(ctx, accountID, amount, fmt.Sprintf("BNPL deposit refund for order %s", orderNumber), orderNumber+"-REFUND"); err != nil {
		slog.Error("deposit refund failed", "error", err)
	}
}

func (s *OrderService) handlePaymentFlow(ctx context.Context, order *model.Order, customerID string) {
	_, err := s.paymentClient.InitiatePayment(ctx, client.InitiatePaymentRequest{
		OrderID:     order.ID.String(),
		Amount:      order.TotalAmount,
		PaymentType: string(order.PaymentType),
		Reference:   order.OrderNumber,
		CustomerID:  customerID,
	})
	if err != nil {
		slog.Error("payment initiation failed", "error", err, "orderId", order.ID)
	}
}

func (s *OrderService) GetOrders(ctx context.Context, tenantID string, userID uuid.UUID, page, size int) ([]model.OrderResponse, int64, error) {
	orders, total, err := s.orderRepo.FindByUser(ctx, tenantID, userID, page, size)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]model.OrderResponse, len(orders))
	for i := range orders {
		resp[i] = orders[i].ToResponse(nil, nil)
	}
	return resp, total, nil
}

func (s *OrderService) GetOrder(ctx context.Context, id uuid.UUID) (*model.OrderResponse, error) {
	order, err := s.orderRepo.FindByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFoundResource("Order", id.String())
		}
		return nil, err
	}
	items, _ := s.orderRepo.FindOrderItems(ctx, id)
	events, _ := s.orderRepo.FindDeliveryEvents(ctx, id)
	resp := order.ToResponse(items, events)
	return &resp, nil
}

type UpdateStatusRequest struct {
	Status      model.OrderStatus `json:"status"`
	Description *string           `json:"description"`
	Location    *string           `json:"location"`
}

func (s *OrderService) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, req UpdateStatusRequest) (*model.OrderResponse, error) {
	order, err := s.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFoundResource("Order", orderID.String())
		}
		return nil, err
	}

	if err := s.orderRepo.UpdateStatus(ctx, orderID, req.Status); err != nil {
		return nil, err
	}

	// Map status to delivery event type.
	var eventType model.DeliveryEventType
	switch req.Status {
	case model.StatusProcessing:
		eventType = model.EventProcessing
	case model.StatusShipped:
		eventType = model.EventInTransit
	case model.StatusDelivered:
		eventType = model.EventDelivered
	default:
		eventType = model.DeliveryEventType(string(req.Status))
	}

	evt := &model.DeliveryEvent{
		ID:          uuid.New(),
		OrderID:     orderID,
		EventType:   eventType,
		Description: req.Description,
		Location:    req.Location,
	}
	_ = s.orderRepo.CreateDeliveryEventNoTx(ctx, evt)

	// Publish events for certain status changes.
	switch req.Status {
	case model.StatusShipped:
		s.publisher.PublishOrderShipped(order.TenantID, order.ID, order.OrderNumber)
	case model.StatusDelivered:
		s.publisher.PublishOrderDelivered(order.TenantID, order.ID, order.OrderNumber)
	}

	return s.GetOrder(ctx, orderID)
}
