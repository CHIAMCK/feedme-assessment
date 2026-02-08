package main

import (
	"fmt"
	"os"
	"se-take-home-assignment/internal/controller"
	"se-take-home-assignment/internal/logger"
)

func main() {
	log := logger.New()
	ctrl := controller.NewOrderController(log)

	// Simulate the order management system
	// This demonstrates VIP priority and multiple VIP orders scenario

	// Step 1: Add one bot first
	ctrl.AddBot()

	// Step 2: Create Normal Order first, then VIP Order
	// This demonstrates VIP priority - VIP should be picked up first
	ctrl.CreateNormalOrder()  // Order #1001 (Normal)
	ctrl.CreateVIPOrder()     // Order #1002 (VIP) - Should be picked up FIRST!

	// Wait to see VIP order gets processed first
	ctrl.Wait(12 * 1000) // Wait 12 seconds for VIP order to complete

	// Step 3: Demonstrate multiple VIP orders scenario
	// Create multiple VIP orders - they should queue behind existing VIP orders
	ctrl.CreateVIPOrder()     // Order #1003 (VIP) - Will queue after #1002
	ctrl.CreateVIPOrder()     // Order #1004 (VIP) - Will queue after #1003
	ctrl.CreateNormalOrder()  // Order #1005 (Normal) - Will queue after all VIPs
	ctrl.CreateNormalOrder()  // Order #1006 (Normal) - Will queue after #1005

	// Wait for processing
	ctrl.Wait(12 * 1000) // Wait for next order to complete

	// Step 4: Add another bot to process orders faster
	ctrl.AddBot()

	// Wait for remaining orders to complete
	ctrl.Wait(15 * 1000) // Wait for remaining orders

	// Output final status
	ctrl.PrintStatus()

	// Write output to result.txt
	output := log.GetOutput()
	err := os.WriteFile("scripts/result.txt", []byte(output), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing result.txt: %v\n", err)
		os.Exit(1)
	}
}

