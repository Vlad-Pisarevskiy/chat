package server

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/hub"
	"chatflow/internal/protocol"
	"chatflow/internal/service"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Server struct {
	upgrader websocket.Upgrader
	service  *service.Service
	hub      *hub.Hub
}

type Conn struct {
	ws     *websocket.Conn
	ch     chan protocol.Send
	userID int
}

func New(service *service.Service, hub *hub.Hub) *Server {

	return &Server{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  readBuffer,
			WriteBufferSize: writeBuffer,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
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
	{
		auth.POST("/register", s.Register)
		auth.POST("/login", s.Login)
		auth.POST("/logout", s.Logout)
	}

	ws := r.Group("/ws")
	ws.Use(s.authorization())
	ws.GET("/", s.Run)

	u := r.Group("/users")
	u.Use(s.pageAuthorization())
	u.GET("/", s.Chats)

	chats := r.Group("/chats")
	chats.Use(s.pageAuthorization())
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

	return r
}

func (s *Server) GetChannel(c *gin.Context) {

	handle := c.Query(handleKey)
	if handle == emptyHandle {
		c.Status(http.StatusNotFound)
		return
	}

	channel, err := s.service.GetChannel(c.Request.Context(), handle)
	if err != nil {
		if errors.Is(err, errors1.ErrIncorrectHandle) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Status(http.StatusBadGateway)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"chat_id":     channel.ID,
		"name":        channel.Name,
		"description": channel.Description,
	})
}

func (s *Server) ClearChat(c *gin.Context) {

	userID, ok := c.Get(userIdKey)
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}

	chat := c.Param(chatIdKey)

	chatID, err := strconv.Atoi(chat)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	lastRead, err := s.service.ClearChat(c.Request.Context(), chatID, userID.(int))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	if err = s.SendCleared(userID.(int), chatID, lastRead); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": err.Error(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"up_to_message_id": lastRead,
	})
}

func (s *Server) JoinChannel(c *gin.Context) {

	userID, ok := c.Get(userIdKey)
	if !ok {
		c.Status(http.StatusBadGateway)
		return
	}

	channel := c.Param(chatIdKey)
	channelID, err := strconv.Atoi(channel)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	join := service.JoinChannel{
		UserID:    userID.(int),
		ChannelID: channelID,
	}

	if err = s.service.JoinChannel(c.Request.Context(), join); err != nil {
		//TODO: статус ошибки
		return
	}

	c.Status(http.StatusOK)
}

func (s *Server) DeleteChat(c *gin.Context) {

	userID, ok := c.Get(userIdKey)
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}

	chat := c.Param(chatIdKey)
	if chat == emptyChat {
		c.Status(http.StatusNotFound)
		return
	}

	chatID, err := strconv.Atoi(chat)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}

	secondUserID, err := s.service.GetSecondMember(c.Request.Context(), userID.(int), chatID)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}

	deleteRequest := service.ChatDelete{
		UserID: userID.(int),
		ChatID: chatID,
	}

	if err = s.service.DeleteChat(c.Request.Context(), deleteRequest); err != nil {
		if errors.Is(err, errors1.ErrWrongChatID) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": err.Error(),
			})
		}
		c.Status(http.StatusBadRequest)
		return
	}

	if err = s.SendDeleted(userID.(int), secondUserID, chatID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": err.Error(),
		})
	}

	c.Status(http.StatusNoContent)
}

func (s *Server) DeleteMessage(c *gin.Context) {

	userID, ok := c.Get(userIdKey)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors1.ErrIncorrectData,
		})
		return
	}

	chat := c.Param(peerID)
	if chat == emptyChat {
		c.Status(http.StatusBadRequest)
		return
	}

	chatID, err := strconv.Atoi(chat)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}

	var deleteRequest service.MessageDelete
	if err = c.ShouldBindJSON(&deleteRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
	}

	deleteRequest.ChatID = chatID
	deleteRequest.UserID = userID.(int)

	if err = s.service.DeleteMessage(c.Request.Context(), deleteRequest); err != nil {
		//TODO: проверка на тип ошибки
	}

	c.Status(http.StatusOK)
}

func (s *Server) CreateGroup(c *gin.Context) {

	userID, ok := c.Get(userIdKey)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors1.ErrIncorrectData,
		})
		return
	}

	var group service.GroupCreate
	if err := c.ShouldBindJSON(&group); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors1.ErrIncorrectData,
		})
		return
	}
	group.OwnerID = userID.(int)

	chatID, err := s.service.CreateGroup(c.Request.Context(), group)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"chat_id": chatID})
}

func (s *Server) CreateChannel(c *gin.Context) {

	ownerID, ok := c.Get(userIdKey)
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}

	var channel service.ChannelCreates
	if err := c.ShouldBindBodyWithJSON(&channel); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	chatID, err := s.service.CreateChannel(c.Request.Context(), ownerID.(int), channel)
	if err != nil {
		if errors.Is(err, errors1.ErrNameInUse) {
			c.JSON(http.StatusConflict, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusBadGateway, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"chat_id": chatID,
	})
}

func (s *Server) GetPeer(c *gin.Context) {

	userID, ok := c.Get(userIdKey)
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}

	peer := c.Query(peerID)
	if peer == emptyPeer {
		c.Status(http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(c.Query(peerID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
	}

	chatID, exist, err := s.service.FindChat(c.Request.Context(), userID.(int), id)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": err.Error(),
		})
	}

	if !exist {
		c.Status(http.StatusNotFound)
		return
	}

	if chatID == nullID {
		c.Status(http.StatusNotFound)
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

	userID, ok := c.Get(userIdKey)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": errors1.ErrUserDoesntExist,
		})
		return
	}

	users, err := s.service.GetUsersExcept(c.Request.Context(), userID.(int))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": err.Error(),
		})
		return
	}

	me, err := s.service.FindUserByID(c.Request.Context(), userID.(int))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": err.Error(),
		})
		return
	}

	groups, err := s.service.GetGroups(c.Request.Context(), userID.(int))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": err.Error(),
		})
		return
	}

	channels, err := s.service.GetChannels(c.Request.Context(), userID.(int))

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

	token, err := c.Cookie(tokenKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
	}

	if err = s.service.Logout(c.Request.Context(), token); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
	}

	c.Status(http.StatusOK)
}

func (s *Server) LoadMessages(c *gin.Context) {

	userFrom, ok := c.Get(userIdKey)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "unknown user",
		})
		return
	}

	chat := c.Param(chatIdKey)
	chatID, err := strconv.Atoi(chat)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "unknown user id",
		})
		return
	}

	messages, err := s.service.LoadMessages(c.Request.Context(), chatID, userFrom.(int))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, messages)
}

func (s *Server) Register(c *gin.Context) {

	var registerRequest RegisterRequest
	if err := c.ShouldBind(&registerRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors1.ErrIncorrectData,
		})
		return
	}

	request := service.RegisterInput{
		Name:     registerRequest.Name,
		Login:    registerRequest.Login,
		Password: registerRequest.Password,
	}

	if err := s.service.RegisterUser(c.Request.Context(), request); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": "registered",
	})
}

func (s *Server) Login(c *gin.Context) {

	var request LoginRequest
	if err := c.ShouldBind(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid login data",
		})
		return
	}

	user, token, err := s.service.LoginUser(c.Request.Context(),
		request.Login,
		request.Password,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error:": err.Error(),
		})
		return
	}

	// Устанавливает, кто может переходить на сайт по ссылкам
	c.SetSameSite(http.SameSiteLaxMode)

	// Задаем куки. Path определяет путь по которому будут работать куки. Secure определяет доступность для не https соединений
	c.SetCookie(tokenKey, token, cookieMaxAge, "/", "", true, true)

	c.JSON(http.StatusOK, gin.H{
		"successful login": fmt.Sprintf("welcome, %s", user.Login),
	})
}

func (s *Server) Run(c *gin.Context) {

	userID, ok := c.Get(userIdKey)
	if !ok {
		log.Println("unexpected error")
		return
	}

	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println(err)
		return
	}

	id := userID.(int)

	sess := newSession(id, conn, s.hub, s.service)
	sess.handle()
}

func (s *Server) authorization() gin.HandlerFunc {
	return func(c *gin.Context) {

		token, err := c.Cookie(tokenKey)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "missing token",
			})
			c.Abort()
			return
		}

		id, err := s.service.CheckToken(c.Request.Context(), token)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "authorization error",
			})
			c.Abort()
			return
		}

		c.Set(userIdKey, id)
		c.Next()
	}
}

func (s *Server) pageAuthorization() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(tokenKey)
		if err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		id, err := s.service.CheckToken(c.Request.Context(), token)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "authorization error",
			})
			c.Abort()
			return
		}

		c.Set(userIdKey, id)
		c.Next()
	}
}

func (s *Server) SendDeleted(userID, secondUserID, chatID int) error {

	removed := protocol.Removed{ChatID: chatID}
	payload, err := json.Marshal(removed)
	if err != nil {
		return err
	}

	data := &protocol.Data{
		Type:    "chat_removed",
		Payload: payload,
	}

	s.hub.Send([]int{userID, secondUserID}, data)

	return nil
}

func (s *Server) SendCleared(userID, chatID, upToMessage int) error {

	cleared := protocol.Cleared{
		ChatID:        chatID,
		UpToMessageID: upToMessage,
	}

	payload, err := json.Marshal(cleared)
	if err != nil {
		return err
	}

	data := &protocol.Data{
		Type:    "chat_cleared",
		Payload: payload,
	}

	s.hub.Send([]int{userID}, data)

	return nil
}
