package auth

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/service"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {

	return &Handler{service: service}
}

func (h *Handler) RegisterRouter() *gin.Engine {

	r := gin.Default()

	r.LoadHTMLFiles("web/login.html", "web/register.html")

	r.GET("/register", h.Registration)
	r.GET("/login", h.Authorization)

	auth := r.Group("/auth")
	auth.Use(jsonOnly())
	{
		auth.POST("/register", h.Register)
		auth.POST("/login", h.Login)
		auth.POST("/logout", h.Logout)
	}
}

func (h *Handler) Registration(c *gin.Context) {
	c.HTML(http.StatusOK, "register.html", nil)
}

func (h *Handler) Authorization(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", nil)
}

func (h *Handler) Register(c *gin.Context) {

	var registerRequest RegisterRequest
	if err := c.ShouldBindJSON(&registerRequest); err != nil {
		s.fail(c, errors1.ErrIncorrectData)
		return
	}

	request := service.RegisterInput{
		Name:     registerRequest.Name,
		Login:    registerRequest.Login,
		Password: registerRequest.Password,
	}

	if err := h.service.RegisterUser(c.Request.Context(), request); err != nil {
		h.fail(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{})
}

func (h *Handler) Login(c *gin.Context) {

	var request LoginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		s.fail(c, errors1.ErrIncorrectData)
		return
	}

	_, token, err := s.service.LoginUser(c.Request.Context(),
		request.Login,
		request.Password,
	)
	if err != nil {
		h.fail(c, err)
		return
	}

	// Устанавливает, кто может переходить на сайт по ссылкам
	c.SetSameSite(http.SameSiteLaxMode)

	// Задаем куки. Path определяет путь по которому будут работать куки. Secure определяет доступность для не https соединений
	c.SetCookie(tokenKey, token, cookieMaxAge, "/", "", true, true)

	c.JSON(http.StatusOK, gin.H{})
}

func (h *Handler) Logout(c *gin.Context) {

	// Без куки выходить уже не из чего, а результат тот же — поэтому тоже 200.
	if token, err := c.Cookie(tokenKey); err == nil {
		if err = h.service.Logout(c.Request.Context(), token); err != nil {
			return
		}
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(tokenKey, emptyToken, removeCookieAge, "/", "", true, true)

	c.Status(http.StatusOK)
}

func jsonOnly() gin.HandlerFunc {
	return func(c *gin.Context) {

		if c.Request.Method == http.MethodPost && c.Request.ContentLength != 0 {
			mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
			if err != nil || mediaType != jsonMediaType {
				c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{
					"error": notJSONText,
				})
				return
			}
		}

		c.Next()
	}
}
