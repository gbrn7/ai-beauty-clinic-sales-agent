package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/raygbrn/ai-sales-agent-go/internal/buffer"
	"github.com/raygbrn/ai-sales-agent-go/internal/config"
	"github.com/raygbrn/ai-sales-agent-go/internal/memory"
	"github.com/raygbrn/ai-sales-agent-go/internal/model"
	"github.com/raygbrn/ai-sales-agent-go/internal/storage"
	"github.com/raygbrn/ai-sales-agent-go/internal/whatsapp"
)

type Dependencies struct {
	Config   *config.Config
	DB       *gorm.DB
	Buffer   *buffer.DebounceBuffer
	Storage  storage.StorageService
	WhatsApp *whatsapp.WhatsAppClient
	Memory   *memory.Memory
}

func RegisterRoutes(r *gin.Engine, deps *Dependencies) {
	// Root and Health
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/dashboard")
	})

	r.GET("/health", func(c *gin.Context) {
		bufferSeconds := 0
		if deps.Config != nil {
			bufferSeconds = deps.Config.BufferSeconds
		}
		c.JSON(http.StatusOK, gin.H{
			"status":         "ok",
			"buffer_seconds": bufferSeconds,
		})
	})

	// Webhook for Evolution API
	r.POST("/webhook", func(c *gin.Context) {
		var payload map[string]any
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}

		incoming := whatsapp.ExtractIncoming(payload)
		if incoming == nil {
			c.JSON(http.StatusOK, gin.H{"status": "ignored"})
			return
		}

		if deps.Buffer != nil {
			count := deps.Buffer.Add(incoming)
			c.JSON(http.StatusOK, gin.H{"status": "buffered", "pending": count})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "buffered", "pending": 1})
	})

	// Media Proxy
	r.GET("/media/:fname", func(c *gin.Context) {
		fname := filepath.Base(c.Param("fname"))
		if deps.Storage == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Storage tidak dikonfigurasi."})
			return
		}
		data, mime, err := deps.Storage.GetMedia(fname)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Gambar tidak ditemukan."})
			return
		}
		c.Header("Cache-Control", "public, max-age=86400")
		c.Data(http.StatusOK, mime, data)
	})

	// Dashboard HTML
	r.GET("/dashboard", func(c *gin.Context) {
		htmlBytes, err := os.ReadFile("templates/dashboard.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "templates/dashboard.html not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", htmlBytes)
	})

	// Dashboard Protected API group
	api := r.Group("/dashboard/api", func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if deps.Config == nil || deps.Config.DashboardAPIKey == "" || key != deps.Config.DashboardAPIKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API key dashboard salah."})
			return
		}
		c.Next()
	})

	api.GET("/config", func(c *gin.Context) {
		company := ""
		aiName := ""
		if deps.Config != nil {
			company = deps.Config.CompanyName
			aiName = deps.Config.AIName
		}
		c.JSON(http.StatusOK, gin.H{
			"company": company,
			"ai_name": aiName,
		})
	})

	api.GET("/conversations", func(c *gin.Context) {
		var msgs []model.ConversationMessage
		deps.DB.Order("created_at DESC").Find(&msgs)

		var contacts []model.Contact
		deps.DB.Find(&contacts)
		contactMap := make(map[string]model.Contact)
		for _, ct := range contacts {
			contactMap[ct.Phone] = ct
		}

		var bookings []model.Booking
		deps.DB.Where("status != ?", "cancelled").Find(&bookings)
		bookingPhones := make(map[string]bool)
		for _, b := range bookings {
			bookingPhones[b.Phone] = true
		}

		lastMsg := make(map[string]model.ConversationMessage)
		var order []string
		for _, m := range msgs {
			if _, exists := lastMsg[m.Phone]; !exists {
				lastMsg[m.Phone] = m
				order = append(order, m.Phone)
			}
		}

		result := make([]gin.H, 0)
		for _, phone := range order {
			m := lastMsg[phone]
			ct, hasCt := contactMap[phone]
			name := phone
			if hasCt && ct.Name != nil && *ct.Name != "" {
				name = *ct.Name
			}
			aiEnabled := true
			if hasCt {
				aiEnabled = ct.AiEnabled
			}
			result = append(result, gin.H{
				"phone":       phone,
				"name":        name,
				"last_text":   m.Text,
				"last_role":   m.Role,
				"last_time":   m.CreatedAt.Format(time.RFC3339),
				"ai_enabled":  aiEnabled,
				"handover":    !aiEnabled,
				"has_booking": bookingPhones[phone],
			})
		}
		c.JSON(http.StatusOK, result)
	})

	api.GET("/conversations/:phone", func(c *gin.Context) {
		phone := c.Param("phone")
		var msgs []model.ConversationMessage
		deps.DB.Where("phone = ?", phone).Order("created_at ASC").Find(&msgs)

		var contact model.Contact
		hasContact := deps.DB.Where("phone = ?", phone).First(&contact).Error == nil

		var bookings []model.Booking
		deps.DB.Preload("Treatment").Where("phone = ?", phone).Order("booking_date DESC").Find(&bookings)

		name := phone
		aiEnabled := true
		isReturning := false
		if hasContact {
			if contact.Name != nil && *contact.Name != "" {
				name = *contact.Name
			}
			aiEnabled = contact.AiEnabled
			isReturning = contact.IsReturning
		}

		msgList := make([]gin.H, 0)
		for _, m := range msgs {
			msgList = append(msgList, gin.H{
				"role": m.Role,
				"text": m.Text,
				"time": m.CreatedAt.Format(time.RFC3339),
			})
		}

		bList := make([]gin.H, 0)
		for _, b := range bookings {
			bList = append(bList, gin.H{
				"code":      b.Code,
				"treatment": b.Treatment.Name,
				"date":      b.BookingDate,
				"time":      b.BookingTime,
				"status":    b.Status,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"phone":        phone,
			"name":         name,
			"ai_enabled":   aiEnabled,
			"is_returning": isReturning,
			"messages":     msgList,
			"bookings":     bList,
		})
	})

	api.POST("/conversations/:phone/ai", func(c *gin.Context) {
		phone := c.Param("phone")
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}

		if deps.Memory != nil {
			_ = deps.Memory.SetAIEnabled(phone, req.Enabled)
		} else {
			var contact model.Contact
			if err := deps.DB.Where("phone = ?", phone).First(&contact).Error; err == nil {
				deps.DB.Model(&contact).Update("ai_enabled", req.Enabled)
			} else {
				deps.DB.Model(&model.Contact{}).Create(map[string]any{
					"phone":      phone,
					"ai_enabled": req.Enabled,
				})
			}
		}
		c.JSON(http.StatusOK, gin.H{"phone": phone, "ai_enabled": req.Enabled})
	})

	api.POST("/conversations/:phone/send", func(c *gin.Context) {
		phone := c.Param("phone")
		var req struct {
			Text string `json:"text"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Text) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Pesan kosong."})
			return
		}

		if deps.WhatsApp != nil {
			go deps.WhatsApp.SendReply(phone, req.Text)
		}
		if deps.Memory != nil {
			_ = deps.Memory.Add(phone, "model", req.Text)
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	api.GET("/bookings", func(c *gin.Context) {
		rangeFilter := c.DefaultQuery("range", "all")
		statusFilter := c.DefaultQuery("status", "all")
		query := c.DefaultQuery("q", "")

		var allBookings []model.Booking
		deps.DB.Preload("Treatment").Order("booking_date DESC, booking_time ASC").Find(&allBookings)

		today := time.Now().Format("2006-01-02")
		stats := gin.H{
			"today":     0,
			"total":     len(allBookings),
			"active":    0,
			"completed": 0,
		}

		for _, b := range allBookings {
			if b.BookingDate == today && b.Status != "cancelled" {
				stats["today"] = stats["today"].(int) + 1
			}
			if b.Status == "pending" || b.Status == "confirmed" {
				stats["active"] = stats["active"].(int) + 1
			}
			if b.Status == "completed" {
				stats["completed"] = stats["completed"].(int) + 1
			}
		}

		daysMap := map[string]int{"7": 7, "30": 30, "90": 90}
		todayDate, _ := time.Parse("2006-01-02", today)

		items := make([]gin.H, 0)
		for _, b := range allBookings {
			if statusFilter != "all" && b.Status != statusFilter {
				continue
			}
			if rangeFilter == "today" && b.BookingDate != today {
				continue
			}
			if d, ok := daysMap[rangeFilter]; ok {
				bDate, err := time.Parse("2006-01-02", b.BookingDate)
				if err == nil {
					minDate := todayDate.AddDate(0, 0, -d)
					maxDate := todayDate.AddDate(0, 0, d)
					if bDate.Before(minDate) || bDate.After(maxDate) {
						continue
					}
				}
			}
			if query != "" {
				ql := strings.ToLower(query)
				if !strings.Contains(strings.ToLower(b.CustomerName), ql) && !strings.Contains(strings.ToLower(b.Treatment.Name), ql) {
					continue
				}
			}
			items = append(items, gin.H{
				"code":          b.Code,
				"customer_name": b.CustomerName,
				"phone":         b.Phone,
				"treatment":     b.Treatment.Name,
				"date":          b.BookingDate,
				"time":          b.BookingTime,
				"status":        b.Status,
			})
		}

		c.JSON(http.StatusOK, gin.H{"stats": stats, "bookings": items})
	})

	api.POST("/bookings/:code/status", func(c *gin.Context) {
		code := c.Param("code")
		var req struct {
			Status string `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Status tidak valid."})
			return
		}

		validStatus := map[string]bool{"pending": true, "confirmed": true, "completed": true, "cancelled": true}
		if !validStatus[req.Status] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Status tidak valid."})
			return
		}

		var b model.Booking
		if err := deps.DB.Where("code = ?", code).First(&b).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Booking tidak ditemukan."})
			return
		}
		b.Status = req.Status
		deps.DB.Save(&b)
		c.JSON(http.StatusOK, gin.H{"code": code, "status": req.Status})
	})
}
