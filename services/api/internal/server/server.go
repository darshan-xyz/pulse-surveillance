package server

import (
	"central-api/internal/auth"
	"central-api/internal/database"
	"central-api/internal/signaling"
	"net/http"
	"time"
	"database/sql"

	"github.com/google/uuid"
	"github.com/gin-gonic/gin"
)

type Server struct {
	router      *gin.Engine
	relay       *signaling.SessionRelay
	pairingMap  map[string]string // pairing_code -> device_id
}

func NewServer() *Server {
	r := gin.Default()
	
	// CORS Middleware
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	relay := signaling.NewSessionRelay()
	
	// API V1 Group
	v1 := r.Group("/v1")
	{
		// Accounts
		v1.POST("/auth/register", registerHandler)
		v1.POST("/auth/login", loginHandler)
		
		// Devices
		v1.POST("/device/pair/request", s.requestPairingHandler)

		// WebSockets (Signaling Relay)
		r.GET("/ws/device", func(c *gin.Context) { relay.HandleDevice(c.Writer, c.Request) })
		r.GET("/ws/client", func(c *gin.Context) {
			tokenString := c.Query("token")
			if tokenString == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token required in query"})
				return
			}
			claims, err := auth.ValidateToken(tokenString)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
				return
			}
			relay.HandleClient(c.Writer, c.Request, claims.UserID)
		})
		
		// Authenticated Routes
		protected := v1.Group("/")
		protected.Use(authMiddleware())
		{
			protected.GET("/homes", getHomesHandler)
			protected.GET("/devices", getDevicesHandler)
			protected.POST("/homes", createHomeHandler)
			protected.POST("/pairing", s.pairDeviceHandler) // Pair a new HSS
		}
	}

	return &Server{router: r, relay: relay, pairingMap: make(map[string]string)}
}

func (s *Server) Start(addr string) error {
	return s.router.Run(addr)
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.GetHeader("Authorization")
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			return
		}

		// Typically "Bearer <token>"
		if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
			tokenString = tokenString[7:]
		}

		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Next()
	}
}

func registerHandler(c *gin.Context) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	userID := uuid.New().String()
	// In production, hash the password!
	_, err := database.DB.Exec("INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)", userID, req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User registered successfully", "user_id": userID})
}

func loginHandler(c *gin.Context) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	var userID, hash string
	err := database.DB.QueryRow("SELECT id, password_hash FROM users WHERE email = $1", req.Email).Scan(&userID, &hash)
	if err == sql.ErrNoRows || hash != req.Password {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	token, _ := auth.GenerateToken(userID)
	c.JSON(http.StatusOK, gin.H{"token": token, "user_id": userID})
}

func getHomesHandler(c *gin.Context) {
	userID := c.GetString("user_id")
	
	rows, err := database.DB.Query(`
		SELECT h.id, h.name 
		FROM homes h 
		JOIN home_members hm ON h.id = hm.home_id 
		WHERE hm.user_id = $1`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var homes []map[string]interface{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err == nil {
			homes = append(homes, map[string]interface{}{"id": id, "name": name})
		}
	}

	c.JSON(http.StatusOK, gin.H{"homes": homes})
}

func getDevicesHandler(c *gin.Context) {
	userID := c.GetString("user_id")
	
	rows, err := database.DB.Query(`
		SELECT d.id, d.name, d.home_id, d.status
		FROM devices d 
		JOIN home_members hm ON d.home_id = hm.home_id 
		WHERE hm.user_id = $1`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var devices []map[string]interface{}
	for rows.Next() {
		var id, name, homeID, status string
		if err := rows.Scan(&id, &name, &homeID, &status); err == nil {
			devices = append(devices, map[string]interface{}{
				"id": id, 
				"name": name,
				"home_id": homeID,
				"status": status,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"devices": devices})
}

func createHomeHandler(c *gin.Context) {
	userID := c.GetString("user_id")
	var req struct {
		Name string `json:"name"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	homeID := uuid.New().String()
	tx, err := database.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction failed"})
		return
	}
	
	_, err = tx.Exec("INSERT INTO homes (id, name) VALUES ($1, $2)", homeID, req.Name)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create home"})
		return
	}

	_, err = tx.Exec("INSERT INTO home_members (home_id, user_id, role) VALUES ($1, $2, $3)", homeID, userID, "owner")
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link user"})
		return
	}
	
	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"message": "Home created", "home_id": homeID})
}

func (s *Server) requestPairingHandler(c *gin.Context) {
	var req struct {
		DeviceID string `json:"device_id"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// MVP: Generate a simple 6-digit code or static for testing
	code := "123456" 
	s.pairingMap[code] = req.DeviceID

	c.JSON(http.StatusOK, gin.H{"pairing_code": code})
}

func (s *Server) pairDeviceHandler(c *gin.Context) {
	userID := c.GetString("user_id")
	var req struct {
		PairingCode string `json:"pairing_code"`
		HomeID      string `json:"home_id"`
		Name        string `json:"name"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Verify tenancy
	var role string
	err := database.DB.QueryRow("SELECT role FROM home_members WHERE user_id = $1 AND home_id = $2", userID, req.HomeID).Scan(&role)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not a member of this home"})
		return
	}

	deviceID, ok := s.pairingMap[req.PairingCode]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invalid pairing code"})
		return
	}

	// Register device in DB
	_, err = database.DB.Exec("INSERT INTO devices (id, home_id, name, public_key) VALUES ($1, $2, $3, $4)", 
		deviceID, req.HomeID, req.Name, "mock_pub_key_from_pairing")
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register device"})
		return
	}

	// Remove code after successful pairing
	delete(s.pairingMap, req.PairingCode)

	c.JSON(http.StatusOK, gin.H{"message": "Device paired successfully", "device_id": deviceID})
}
