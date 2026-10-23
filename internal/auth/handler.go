package auth

import (
	"context"
	"mime"
	"net/http"

	authserv "chatflow/gen/auth_service/v1"

	"github.com/gin-gonic/gin"
)

type RegisterRequest struct {
	Name     string `json:"name"`
	Login    string `json:"login"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type Handler struct {
	service *Service
	authserv.UnimplementedUserServiceServer
	authserv.UnimplementedTokenServiceServer
}

func NewHandler(service *Service) *Handler {

	return &Handler{service: service}
}

func (h *Handler) GetUsersExcept(ctx context.Context, request *authserv.UserIDRequest) (*authserv.UsersResponse, error) {

	users, err := h.service.GetUserExcept(ctx, int(request.Id))
	if err != nil {
		return nil, err
	}

	return &authserv.UsersResponse{Users: users}, nil
}

func (h *Handler) CheckToken(ctx context.Context, token *authserv.TokenRequest) (*authserv.TokenResponse, error) {

	userID, err := h.service.CheckToken(ctx, token.Token)
	if err != nil {
		return nil, err
	}

	return &authserv.TokenResponse{UserID: int64(userID)}, nil
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

	return r
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
		c.Status(http.StatusBadRequest)
		return
	}

	request := RegisterInput{
		Name:     registerRequest.Name,
		Login:    registerRequest.Login,
		Password: registerRequest.Password,
	}

	if err := h.service.RegisterUser(c.Request.Context(), request); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	c.JSON(http.StatusCreated, gin.H{})
}

func (h *Handler) Login(c *gin.Context) {

	var request LoginRequest
	if err := c.ShouldBindJSON(&request); err != nil {

		return
	}

	_, token, err := h.service.LoginUser(c.Request.Context(),
		request.Login,
		request.Password,
	)
	if err != nil {
		return
	}

	// Устанавливает, кто может переходить на сайт по ссылкам
	c.SetSameSite(http.SameSiteLaxMode)

	// Задаем куки. Path определяет путь по которому будут работать куки. Secure определяет доступность для не https соединений
	c.SetCookie(tokenKey, token, cookieMaxAge, "/", "", true, true)

	c.JSON(http.StatusOK, gin.H{})
}

func (h *Handler) Logout(c *gin.Context) {

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
