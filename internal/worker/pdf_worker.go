package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"invoicefast/internal/cache"
	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"
	"invoicefast/internal/pdf"

	"github.com/redis/go-redis/v9"
)

type PDFWorker struct {
	redis     *cache.RedisCache
	db        *database.DB
	generator *pdf.PDFGenerator
	queueName string
	stopChan  chan struct{}
}

type PDFTask struct {
	InvoiceID   string    `json:"invoice_id"`
	TenantID    string    `json:"tenant_id"`
	InvoiceNum  string    `json:"invoice_number"`
	CreatedAt   time.Time `json:"created_at"`
	Priority    int       `json:"priority"`
	CallbackURL string    `json:"callback_url,omitempty"`
}

type PDFTaskResult struct {
	TaskID    string    `json:"task_id"`
	InvoiceID string    `json:"invoice_id"`
	Status    string    `json:"status"`
	PDFURL    string    `json:"pdf_url,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

func NewPDFWorker(redisCache *cache.RedisCache, db *database.DB, cfg *config.Config) *PDFWorker {
	generator := pdf.NewPDFGenerator("./templates", "./data/pdfs")

	return &PDFWorker{
		redis:     redisCache,
		db:        db,
		generator: generator,
		queueName: "invoicefast:pdf_queue",
		stopChan:  make(chan struct{}),
	}
}

func (w *PDFWorker) Start(ctx context.Context) {
	log.Println("[PDFWorker] Starting worker...")

	go w.processLoop(ctx)
	go w.monitorQueue(ctx)

	log.Println("[PDFWorker] Worker started successfully")
}

func (w *PDFWorker) Stop() {
	log.Println("[PDFWorker] Stopping worker...")
	close(w.stopChan)
}

func (w *PDFWorker) EnqueueTask(ctx context.Context, task *PDFTask) error {
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	score := float64(time.Now().UnixNano())
	if task.Priority > 0 {
		score = float64(task.Priority)*1e15 + score
	}

	err = w.redis.ZAdd(ctx, w.queueName, &redis.Z{
		Score:  score,
		Member: string(taskJSON),
	})

	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	log.Printf("[PDFWorker] Enqueued PDF task for invoice %s", task.InvoiceID)
	return nil
}

func (w *PDFWorker) processLoop(ctx context.Context) {
	for {
		select {
		case <-w.stopChan:
			return
		case <-ctx.Done():
			return
		default:
			w.processNextTask(ctx)
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (w *PDFWorker) processNextTask(ctx context.Context) {
	result, err := w.redis.ZPopMin(ctx, w.queueName, 1)
	if err != nil || len(result) == 0 {
		return
	}

	taskJSON := result[0].Member.(string)

	var task PDFTask
	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		log.Printf("[PDFWorker] Failed to unmarshal task: %v", err)
		return
	}

	w.executeTask(ctx, &task)
}

func (w *PDFWorker) executeTask(ctx context.Context, task *PDFTask) {
	startTime := time.Now()
	result := PDFTaskResult{
		TaskID:    fmt.Sprintf("pdf_%s", task.InvoiceID),
		InvoiceID: task.InvoiceID,
		Status:    "processing",
		StartedAt: startTime,
	}

	log.Printf("[PDFWorker] Processing PDF for invoice %s", task.InvoiceID)

	var invoice models.Invoice
	err := w.db.Preload("Client").Preload("Items").Preload("User").
		First(&invoice, "id = ? AND tenant_id = ?", task.InvoiceID, task.TenantID).Error

	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("invoice not found: %v", err)
		w.completeTask(ctx, task, result)
		return
	}

	pdfData := &pdf.InvoiceData{
		InvoiceNumber: invoice.InvoiceNumber,
		InvoiceDate:   invoice.CreatedAt,
		DueDate:       invoice.DueDate,
		Currency:      invoice.Currency,
		Subtotal:      invoice.Subtotal,
		TaxRate:       invoice.TaxRate,
		TaxAmount:     invoice.TaxAmount,
		Discount:      invoice.Discount,
		Total:         invoice.Total,
		Status:        string(invoice.Status),
		PaidAmount:    invoice.PaidAmount,
		Balance:       invoice.Total - invoice.PaidAmount,
		Notes:         invoice.Notes,
		Terms:         invoice.Terms,
		BrandColor:    invoice.BrandColor,
	}

	// Add KRA compliance data
	if invoice.KRAICN != "" {
		pdfData.KRACompliant = true
		pdfData.ControlNumber = invoice.KRAICN
		pdfData.QRCodeData = invoice.KRAQRCode
	}

	if invoice.Client.ID != "" {
		pdfData.ClientName = invoice.Client.Name
		pdfData.ClientEmail = invoice.Client.Email
		pdfData.ClientPhone = invoice.Client.Phone
		pdfData.ClientAddress = invoice.Client.Address
	}

	if invoice.User.ID != "" {
		pdfData.CompanyName = invoice.User.CompanyName
		pdfData.CompanyEmail = invoice.User.Email
		pdfData.CompanyPhone = invoice.User.Phone
		pdfData.CompanyKRA = invoice.User.KRAPIN
	}

	lineItems := make([]pdf.InvoiceLineItem, len(invoice.Items))
	for i, item := range invoice.Items {
		lineItems[i] = pdf.InvoiceLineItem{
			Description: item.Description,
			Quantity:    item.Quantity,
			Unit:        item.Unit,
			UnitPrice:   item.UnitPrice,
			Total:       item.Total,
		}
	}
	pdfData.Items = lineItems

	output, err := w.generator.GenerateInvoicePDF(pdfData)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("PDF generation failed: %v", err)
		w.completeTask(ctx, task, result)
		return
	}

	// Create tenant-scoped path: /static/uploads/invoices/{tenant_id}/
	pdfDir := fmt.Sprintf("./static/uploads/invoices/%s", task.TenantID)
	pdfFilename := fmt.Sprintf("invoice_%s.pdf", task.InvoiceNum)

	if err := w.savePDF(pdfDir, pdfFilename, output.Content); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to save PDF: %v", err)
		w.completeTask(ctx, task, result)
		return
	}

	result.Status = "completed"
	result.PDFURL = fmt.Sprintf("/static/uploads/invoices/%s/%s", task.TenantID, pdfFilename)
	w.completeTask(ctx, task, result)
}

func (w *PDFWorker) savePDF(dir, filename string, content []byte) error {
	// Ensure tenant directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[PDFWorker] Failed to create directory: %v", err)
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file to disk
	path := fmt.Sprintf("%s/%s", dir, filename)
	if err := os.WriteFile(path, content, 0644); err != nil {
		log.Printf("[PDFWorker] Failed to save PDF: %v", err)
		return fmt.Errorf("failed to save PDF: %w", err)
	}

	log.Printf("[PDFWorker] PDF saved successfully: %s", path)
	return nil
}

func (w *PDFWorker) completeTask(ctx context.Context, task *PDFTask, result PDFTaskResult) {
	result.EndedAt = time.Now()

	log.Printf("[PDFWorker] Task completed: invoice=%s status=%s duration=%v",
		result.InvoiceID, result.Status, result.EndedAt.Sub(result.StartedAt))

	w.db.Model(&models.Invoice{}).Where("id = ?", task.InvoiceID).Updates(map[string]interface{}{
		"pdf_url": result.PDFURL,
	})

	resultJSON, _ := json.Marshal(result)
	w.redis.Set(ctx, fmt.Sprintf("pdf_result:%s", task.InvoiceID), resultJSON, 1*time.Hour)
}

func (w *PDFWorker) monitorQueue(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			count, err := w.redis.ZCard(ctx, w.queueName)
			if err == nil && count > 0 {
				log.Printf("[PDFWorker] Queue depth: %d pending tasks", count)
			}
		}
	}
}

func (w *PDFWorker) GetTaskStatus(ctx context.Context, invoiceID string) (*PDFTaskResult, error) {
	var result PDFTaskResult
	err := w.redis.Get(ctx, fmt.Sprintf("pdf_result:%s", invoiceID), &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (w *PDFWorker) GetQueueStats(ctx context.Context) (int64, error) {
	return w.redis.ZCard(ctx, w.queueName)
}
