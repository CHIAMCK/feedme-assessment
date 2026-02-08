package controller

import (
	"se-take-home-assignment/internal/logger"
	"sync"
	"time"
)

type OrderType int

const (
	OrderTypeNormal OrderType = iota
	OrderTypeVIP
)

type OrderStatus int

const (
	OrderStatusPending OrderStatus = iota
	OrderStatusProcessing
	OrderStatusComplete
)

type Order struct {
	ID       int
	Type     OrderType
	Status   OrderStatus
	BotID    int
	Created  time.Time
	Started  time.Time
	Completed time.Time
}

type BotStatus int

const (
	BotStatusIdle BotStatus = iota
	BotStatusProcessing
)

type Bot struct {
	ID       int
	Status   BotStatus
	Order    *Order
	stopChan chan bool
	mu       sync.Mutex
}

type OrderController struct {
	logger        *logger.Logger
	orders        []*Order
	pendingOrders []*Order
	completedOrders []*Order
	bots          []*Bot
	nextOrderID   int
	nextBotID     int
	mu            sync.Mutex
}

func NewOrderController(log *logger.Logger) *OrderController {
	return &OrderController{
		logger:          log,
		orders:          make([]*Order, 0),
		pendingOrders:   make([]*Order, 0),
		completedOrders: make([]*Order, 0),
		bots:            make([]*Bot, 0),
		nextOrderID:     1001,
		nextBotID:       1,
	}
}

func (oc *OrderController) CreateNormalOrder() {
	oc.mu.Lock()
	orderID := oc.nextOrderID
	oc.nextOrderID++
	oc.mu.Unlock()

	order := &Order{
		ID:      orderID,
		Type:    OrderTypeNormal,
		Status:  OrderStatusPending,
		Created: time.Now(),
	}

	oc.mu.Lock()
	oc.orders = append(oc.orders, order)
	oc.pendingOrders = append(oc.pendingOrders, order)
	oc.mu.Unlock()

	oc.logger.Log("Created Normal Order #%d - Status: PENDING", orderID)
	oc.assignOrdersToBots()
}

func (oc *OrderController) CreateVIPOrder() {
	oc.mu.Lock()
	orderID := oc.nextOrderID
	oc.nextOrderID++
	oc.mu.Unlock()

	order := &Order{
		ID:      orderID,
		Type:    OrderTypeVIP,
		Status:  OrderStatusPending,
		Created: time.Now(),
	}

	oc.mu.Lock()
	oc.orders = append(oc.orders, order)
	
	// Insert VIP order: after all VIP orders, before all normal orders
	insertIndex := 0
	for i, pendingOrder := range oc.pendingOrders {
		if pendingOrder.Type == OrderTypeVIP {
			insertIndex = i + 1
		}
	}
	
	// Insert at the correct position
	if insertIndex >= len(oc.pendingOrders) {
		oc.pendingOrders = append(oc.pendingOrders, order)
	} else {
		oc.pendingOrders = append(oc.pendingOrders[:insertIndex], append([]*Order{order}, oc.pendingOrders[insertIndex:]...)...)
	}
	oc.mu.Unlock()

	oc.logger.Log("Created VIP Order #%d - Status: PENDING", orderID)
	oc.assignOrdersToBots()
}

func (oc *OrderController) AddBot() {
	oc.mu.Lock()
	botID := oc.nextBotID
	oc.nextBotID++
	oc.mu.Unlock()

	bot := &Bot{
		ID:       botID,
		Status:   BotStatusIdle,
		stopChan: make(chan bool, 1),
	}

	oc.mu.Lock()
	oc.bots = append(oc.bots, bot)
	oc.mu.Unlock()

	oc.logger.Log("Bot #%d created - Status: ACTIVE", botID)
	oc.assignOrdersToBots()
}

func (oc *OrderController) RemoveBot() {
	oc.mu.Lock()
	if len(oc.bots) == 0 {
		oc.mu.Unlock()
		return
	}

	// Remove the newest bot (last in the slice)
	botIndex := len(oc.bots) - 1
	bot := oc.bots[botIndex]
	oc.bots = oc.bots[:botIndex]
	oc.mu.Unlock()

	bot.mu.Lock()
	if bot.Status == BotStatusProcessing && bot.Order != nil {
		// Stop processing and return order to pending
		bot.stopChan <- true
		order := bot.Order
		order.Status = OrderStatusPending
		order.BotID = 0
		
		oc.mu.Lock()
		// Re-insert order back to pending queue with proper priority
		if order.Type == OrderTypeVIP {
			insertIndex := 0
			for i, pendingOrder := range oc.pendingOrders {
				if pendingOrder.Type == OrderTypeVIP {
					insertIndex = i + 1
				}
			}
			if insertIndex >= len(oc.pendingOrders) {
				oc.pendingOrders = append(oc.pendingOrders, order)
			} else {
				oc.pendingOrders = append(oc.pendingOrders[:insertIndex], append([]*Order{order}, oc.pendingOrders[insertIndex:]...)...)
			}
		} else {
			oc.pendingOrders = append(oc.pendingOrders, order)
		}
		oc.mu.Unlock()
		
		oc.logger.Log("Bot #%d destroyed while processing Order #%d - Order returned to PENDING", bot.ID, order.ID)
	} else {
		oc.logger.Log("Bot #%d destroyed while IDLE", bot.ID)
	}
	bot.mu.Unlock()

	oc.assignOrdersToBots()
}

func (oc *OrderController) assignOrdersToBots() {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	// Find idle bots
	for _, bot := range oc.bots {
		bot.mu.Lock()
		if bot.Status == BotStatusIdle && len(oc.pendingOrders) > 0 {
			// Get the first pending order (highest priority)
			order := oc.pendingOrders[0]
			oc.pendingOrders = oc.pendingOrders[1:]

			// Assign order to bot
			bot.Order = order
			bot.Status = BotStatusProcessing
			order.Status = OrderStatusProcessing
			order.BotID = bot.ID
			order.Started = time.Now()

			oc.logger.Log("Bot #%d picked up %s Order #%d - Status: PROCESSING", 
				bot.ID, oc.getOrderTypeString(order.Type), order.ID)

			// Start processing in a goroutine
			go oc.processOrder(bot)
		}
		bot.mu.Unlock()
	}
}

func (oc *OrderController) processOrder(bot *Bot) {
	// Process for 10 seconds
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	startTime := time.Now()
	duration := 10 * time.Second

	for {
		select {
		case <-bot.stopChan:
			// Bot was removed, stop processing
			return
		case <-ticker.C:
			if time.Since(startTime) >= duration {
				// Order completed
				bot.mu.Lock()
				if bot.Order != nil {
					order := bot.Order
					order.Status = OrderStatusComplete
					order.Completed = time.Now()
					processingTime := order.Completed.Sub(order.Started)
					
					oc.mu.Lock()
					oc.completedOrders = append(oc.completedOrders, order)
					oc.mu.Unlock()

				oc.logger.Log("Bot #%d completed %s Order #%d - Status: COMPLETE (Processing time: %ds)", 
					bot.ID, oc.getOrderTypeString(order.Type), order.ID, int(processingTime.Seconds()))

				bot.Order = nil
				bot.Status = BotStatusIdle
			}
			bot.mu.Unlock()

			// Try to assign another order
			oc.mu.Lock()
			hasPendingOrders := len(oc.pendingOrders) > 0
			oc.mu.Unlock()
			
			oc.assignOrdersToBots()
			
			// If no pending orders after assignment, log idle status
			if !hasPendingOrders {
				bot.mu.Lock()
				if bot.Status == BotStatusIdle {
					oc.logger.Log("Bot #%d is now IDLE - No pending orders", bot.ID)
				}
				bot.mu.Unlock()
			}
			return
			}
		}
	}
}

func (oc *OrderController) Wait(milliseconds int) {
	time.Sleep(time.Duration(milliseconds) * time.Millisecond)
	
	// Check if any bots became idle and need new orders
	oc.assignOrdersToBots()
}

func (oc *OrderController) PrintStatus() {
	oc.mu.Lock()
	defer oc.mu.Unlock()

	vipCount := 0
	normalCount := 0
	for _, order := range oc.completedOrders {
		if order.Type == OrderTypeVIP {
			vipCount++
		} else {
			normalCount++
		}
	}

	oc.logger.Log("")
	oc.logger.Log("Final Status:")
	oc.logger.Log("- Total Orders Processed: %d (%d VIP, %d Normal)", 
		len(oc.completedOrders), vipCount, normalCount)
	oc.logger.Log("- Orders Completed: %d", len(oc.completedOrders))
	oc.logger.Log("- Active Bots: %d", len(oc.bots))
	oc.logger.Log("- Pending Orders: %d", len(oc.pendingOrders))
}

func (oc *OrderController) getOrderTypeString(orderType OrderType) string {
	if orderType == OrderTypeVIP {
		return "VIP"
	}
	return "Normal"
}

