package server

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/hub"
	"chatflow/internal/protocol"
	"chatflow/internal/service"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Server struct {
	upgrader websocket.Upgrader
	service  *service.Service
	hub      *hub.Hub
}

func New(service *service.Service, hub *hub.Hub) *Server {

	return &Server{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  readBuffer,
			WriteBufferSize: writeBuffer,
			CheckOrigin:     sameOrigin,
		},
		service: service,
		hub:     hub}
}

func (s *Server) GetRouter() *gin.Engine {

	r := gin.Default()

	r.LoadHTMLFiles("web/login.html", "web/register.html", "web/users.html")

	r.GET("/register", s.Registration)
	r.GET("/login", s.Authorization)

	auth := r.Group("/auth")
	auth.Use(jsonOnly())
	{
		auth.POST("/register", s.Register)
		auth.POST("/login", s.Login)
		auth.POST("/logout", s.Logout)
	}

	ws := r.Group("/ws")
	ws.Use(s.apiAuthorization())
	ws.GET("/", s.Run)

	u := r.Group("/users")
	u.Use(s.pageAuthorization())
	u.GET("/", s.Chats)

	chats := r.Group("/chats")
	chats.Use(s.apiAuthorization(), jsonOnly())
	{
		chats.GET("/direct", s.GetPeer)
		chats.GET("/:chatID/messages", s.LoadMessages)
		chats.GET("/channels/", s.GetChannel)

		chats.POST("/:chatID/join", s.JoinChannel)
		chats.POST("/:chatID/delete", s.DeleteChat)
		chats.POST("/:chatID/clear", s.ClearChat)

		chats.POST("/:chatID/messages/delete", s.DeleteMessage)
		chats.POST("/group", s.CreateGroup)
		chats.POST("/channels", s.CreateChannel)
	}

	// Картинка приходит как multipart, поэтому jsonOnly здесь не нужен.
	images := r.Group("/chats", s.apiAuthorization())
	images.POST("/images", s.SendImage)

	r.GET("/images/:imageID", s.apiAuthorization(), s.GetImage)

	return r
}

func (s *Server) GetChannel(c *gin.Context) {

	handle := c.Query(handleKey)
	if handle == emptyHandle {
		s.fail(c, errors1.ErrChatNotFound)
		return
	}

	channel, err := s.service.GetChannel(c.Request.Context(), handle)
	if err != nil {
		s.fail(c, err)
		return
	}

	response := gin.H{
		"chat_id": channel.ID,
		"name":    channel.Name,
		"handle":  handle,
	}

	// Нет описания — нет и поля: у «нет описания» одно представление.
	if channel.Description != emptyDescription {
		response["description"] = channel.Description
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) ClearChat(c *gin.Context) {

	userID := c.GetInt(userIdKey)

	chatID, ok := chatParam(c)
	if !ok {
		return
	}

	upTo, err := s.service.ClearChat(c.Request.Context(), chatID, userID)
	if err != nil {
		s.fail(c, err)
		return
	}

	s.broadcast([]int{userID}, chatClearedType, protocol.Cleared{
		ChatID:        chatID,
		UpToMessageID: upTo,
	})

	c.JSON(http.StatusOK, gin.H{
		"up_to_message_id": upTo,
	})
}

func (s *Server) JoinChannel(c *gin.Context) {

	userID := c.GetInt(userIdKey)

	channelID, ok := chatParam(c)
	if !ok {
		return
	}

	join := service.JoinChannel{
		UserID:    userID,
		ChannelID: channelID,
	}

	joined, err := s.service.JoinChannel(c.Request.Context(), join)
	if err != nil {
		s.fail(c, err)
		return
	}

	s.broadcast([]int{userID}, chatAddedType, protocol.ChatAdded{
		ChatID: joined.ID,
		Kind:   protocol.KindChannel,
		Name:   joined.Name,
		Role:   joined.Role,
	})

	c.JSON(http.StatusOK, gin.H{
		"chat_id": joined.ID,
		"name":    joined.Name,
		"role":    joined.Role,
	})
}

func (s *Server) DeleteChat(c *gin.Context) {

	userID := c.GetInt(userIdKey)

	chatID, ok := chatParam(c)
	if !ok {
		return
	}

	deleteRequest := service.ChatDelete{
		UserID: userID,
		ChatID: chatID,
	}

	members, err := s.service.DeleteChat(c.Request.Context(), deleteRequest)
	if err != nil {
		s.fail(c, err)
		return
	}

	s.broadcast(members, chatRemovedType, protocol.Removed{ChatID: chatID})

	c.Status(http.StatusNoContent)
}

func (s *Server) DeleteMessage(c *gin.Context) {

	chatID, ok := chatParam(c)
	if !ok {
		return
	}

	var deleteRequest service.MessageDelete
	if err := c.ShouldBindJSON(&deleteRequest); err != nil {
		s.fail(c, errors1.ErrIncorrectData)
		return
	}

	deleteRequest.ChatID = chatID
	deleteRequest.UserID = c.GetInt(userIdKey)

	if err := s.service.DeleteMessage(c.Request.Context(), deleteRequest); err != nil {
		//TODO: проверка на тип ошибки
		s.fail(c, err)
		return
	}

	c.Status(http.StatusOK)
}

func (s *Server) CreateGroup(c *gin.Context) {

	var group service.GroupCreate
	if err := c.ShouldBindJSON(&group); err != nil {
		s.fail(c, errors1.ErrIncorrectData)
		return
	}
	group.OwnerID = c.GetInt(userIdKey)

	created, err := s.service.CreateGroup(c.Request.Context(), group)
	if err != nil {
		s.fail(c, err)
		return
	}

	s.broadcast(created.Members, chatAddedType, protocol.ChatAdded{
		ChatID: created.ChatID,
		Kind:   protocol.KindGroup,
		Name:   created.Name,
	})

	c.JSON(http.StatusCreated, gin.H{"chat_id": created.ChatID})
}

func (s *Server) CreateChannel(c *gin.Context) {

	ownerID := c.GetInt(userIdKey)

	var channel service.ChannelCreates
	if err := c.ShouldBindJSON(&channel); err != nil {
		s.fail(c, errors1.ErrIncorrectData)
		return
	}

	chatID, name, err := s.service.CreateChannel(c.Request.Context(), ownerID, channel)
	if err != nil {
		s.fail(c, err)
		return
	}

	s.broadcast([]int{ownerID}, chatAddedType, protocol.ChatAdded{
		ChatID: chatID,
		Kind:   protocol.KindChannel,
		Name:   name,
		Role:   ownerRole,
	})

	c.JSON(http.StatusCreated, gin.H{
		"chat_id": chatID,
	})
}

func (s *Server) GetPeer(c *gin.Context) {

	userID := c.GetInt(userIdKey)

	peer, err := strconv.Atoi(c.Query(peerID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": badPeerText,
		})
		return
	}

	if peer == userID {
		s.fail(c, errors1.ErrPeerIsSelf)
		return
	}

	chatID, exist, err := s.service.FindChat(c.Request.Context(), userID, peer)
	if err != nil {
		s.fail(c, err)
		return
	}

	// Переписки ещё не было — штатный ответ, а не ошибка: чат заведётся на
	// первом сообщении.
	if !exist {
		c.JSON(http.StatusNotFound, gin.H{
			"error": noDirectChatText,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"chat_id": chatID,
	})
}

func (s *Server) Registration(c *gin.Context) {
	c.HTML(http.StatusOK, "register.html", nil)
}

func (s *Server) Authorization(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", nil)
}

func (s *Server) Chats(c *gin.Context) {

	userID := c.GetInt(userIdKey)
	ctx := c.Request.Context()

	users, err := s.service.GetUsersExcept(ctx, userID)
	if err != nil {
		s.fail(c, err)
		return
	}

	me, err := s.service.FindUserByID(ctx, userID)
	if err != nil {
		s.fail(c, err)
		return
	}

	groups, err := s.service.GetGroups(ctx, userID)
	if err != nil {
		s.fail(c, err)
		return
	}

	channels, err := s.service.GetChannels(ctx, userID)
	if err != nil {
		s.fail(c, err)
		return
	}

	online := s.hub.OnlineUsers()

	c.HTML(http.StatusOK, "users.html", gin.H{
		"Users":     users,
		"Groups":    groups,
		"Channels":  channels,
		"Me":        me.Name,
		"OnlineIDs": online,
		"MyID":      me.ID,
	})
}

func (s *Server) Logout(c *gin.Context) {

	// Без куки выходить уже не из чего, а результат тот же — поэтому тоже 200.
	if token, err := c.Cookie(tokenKey); err == nil {
		if err = s.service.Logout(c.Request.Context(), token); err != nil {
			s.fail(c, err)
			return
		}
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(tokenKey, emptyToken, removeCookieAge, "/", "", true, true)

	c.Status(http.StatusOK)
}

func (s *Server) LoadMessages(c *gin.Context) {

	chatID, ok := chatParam(c)
	if !ok {
		return
	}

	messages, err := s.service.LoadMessages(c.Request.Context(), chatID, c.GetInt(userIdKey))
	if err != nil {
		s.fail(c, err)
		return
	}

	c.JSON(http.StatusOK, messages)
}

func (s *Server) Register(c *gin.Context) {

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

	if err := s.service.RegisterUser(c.Request.Context(), request); err != nil {
		s.fail(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{})
}

func (s *Server) Login(c *gin.Context) {

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
		s.fail(c, err)
		return
	}

	// Устанавливает, кто может переходить на сайт по ссылкам
	c.SetSameSite(http.SameSiteLaxMode)

	// Задаем куки. Path определяет путь по которому будут работать куки. Secure определяет доступность для не https соединений
	c.SetCookie(tokenKey, token, cookieMaxAge, "/", "", true, true)

	c.JSON(http.StatusOK, gin.H{})
}

func (s *Server) Run(c *gin.Context) {

	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Клиенту Upgrade уже ответил сам, например 403 на чужой Origin.
		slog.Info("соединение не открыто", "err", err)
		return
	}

	sess := newSession(c.GetInt(userIdKey), conn, s.hub, s.service)
	sess.handle()
}

// apiAuthorization пускает дальше только с живой сессией. Без неё — 401 с
// телом ошибки: по редиректу fetch прошёл бы молча и получил HTML страницы
// входа, а фронт не отличил бы «сессия истекла» от «сервер сломался».
func (s *Server) apiAuthorization() gin.HandlerFunc {
	return func(c *gin.Context) {

		userID, err := s.sessionUser(c)
		if err != nil {
			if errors.Is(err, errors1.ErrInvalidToken) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": unauthorizedText,
				})
				return
			}

			s.fail(c, err)
			c.Abort()
			return
		}

		c.Set(userIdKey, userID)
		c.Next()
	}
}

// pageAuthorization — то же для страниц: без сессии отправляет на вход.
func (s *Server) pageAuthorization() gin.HandlerFunc {
	return func(c *gin.Context) {

		userID, err := s.sessionUser(c)
		if err != nil {
			if !errors.Is(err, errors1.ErrInvalidToken) {
				slog.Error("сессия не проверена", "err", err)
			}

			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		c.Set(userIdKey, userID)
		c.Next()
	}
}

// sessionUser находит пользователя по куке. Нет куки или токен не найден —
// ErrInvalidToken, любая другая ошибка — сбой базы.
func (s *Server) sessionUser(c *gin.Context) (int, error) {

	token, err := c.Cookie(tokenKey)
	if err != nil || token == emptyToken {
		return nullID, errors1.ErrInvalidToken
	}

	return s.service.CheckToken(c.Request.Context(), token)
}

// jsonOnly принимает тело только с Content-Type: application/json. Чужой сайт
// может отправить форму с JSON-подобным телом как text/plain, и preflight при
// этом не случится. Запросы без тела проходят как есть.
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

// sameOrigin не даёт чужому сайту открыть сокет от имени пользователя (CSWSH):
// браузер приложит куку, но Origin будет чужим. Без Origin приходят не
// браузеры, а им чужая кука не достанется.
func sameOrigin(r *http.Request) bool {

	origin := r.Header.Get("Origin")
	if origin == emptyOrigin {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	return strings.EqualFold(u.Host, r.Host)
}

// chatParam читает :chatID. Не число — такого чата быть не может, отвечает 404.
func chatParam(c *gin.Context) (int, bool) {

	chatID, err := strconv.Atoi(c.Param(chatIdKey))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": badChatText,
		})
		return nullID, false
	}

	return chatID, true
}

// fail отвечает на ошибку: известные — своим статусом и текстом для человека,
// остальные — 500 с общим текстом, а подробности только в лог.
func (s *Server) fail(c *gin.Context, err error) {

	f, known := classify(err)
	if !known {
		slog.Error("ошибка обработки запроса", "method", c.Request.Method, "path", c.FullPath(), "err", err)
	}

	c.JSON(f.status, gin.H{
		"error": f.text,
	})
}

// broadcast рассылает кадр на все соединения перечисленных пользователей.
func (s *Server) broadcast(userIDs []int, frameType string, payload any) {

	frame, err := newFrame(frameType, payload)
	if err != nil {
		slog.Error("кадр не собран", "type", frameType, "err", err)
		return
	}

	s.hub.Send(userIDs, frame)
}

func newFrame(frameType string, payload any) (*protocol.Data, error) {

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("кадр %s: %w", frameType, err)
	}

	return &protocol.Data{Type: frameType, Payload: raw}, nil
}
