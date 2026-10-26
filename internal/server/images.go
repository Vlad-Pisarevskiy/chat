package server

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/protocol"
	"chatflow/internal/service"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// SendImage принимает сообщение с картинкой одним multipart-запросом: файл,
// подпись и client_msg_id. Ответ играет роль ack, а всем участникам чата уходит
// кадр message, как после send.
func (s *Server) SendImage(c *gin.Context) {

	userID := c.GetInt(userIdKey)

	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != multipartMediaType {
		c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{
			"error": notMultipartText,
		})
		return
	}

	if c.Request.ContentLength > maxRequestBytes {
		s.fail(c, errors1.ErrImageTooLarge)
		return
	}

	form, err := readImageForm(c)
	if err != nil {
		s.fail(c, err)
		return
	}
	defer func() {
		_ = form.RemoveAll()
	}()

	data, err := readImageFile(form)
	if err != nil {
		s.fail(c, err)
		return
	}

	send := protocol.Send{
		Body:        formValue(form, bodyField),
		ClientMsgID: formValue(form, clientMsgIDField),
	}
	if send.ChatID, err = formID(form, chatIDField); err != nil {
		s.fail(c, err)
		return
	}
	if send.PeerID, err = formID(form, peerID); err != nil {
		s.fail(c, err)
		return
	}

	send, image, err := s.service.PrepareImage(send, userID, data)
	if err != nil {
		s.fail(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), imageWriteTimeout)
	defer cancel()

	if send.PeerID != nullID {
		if send.ChatID, err = ensureDirectChat(ctx, s.service, s.hub, userID, send.PeerID); err != nil {
			s.fail(c, err)
			return
		}
	}

	sent, err := s.service.SendImage(ctx, send, image, userID)
	if err != nil {
		s.fail(c, err)
		return
	}

	response := ImageResponse{
		MessageID: sent.Message.Id,
		ChatID:    sent.Message.ChatID,
		ImageID:   sent.Message.ImageID,
		Time:      sent.Message.Time,
	}

	// Повтор: сообщение разослали, когда сохраняли его в первый раз.
	if sent.Duplicate {
		c.JSON(http.StatusOK, response)
		return
	}

	s.broadcast(sent.Recipients, messageType, sent.Message)

	c.JSON(http.StatusCreated, response)
}

// GetImage отдаёт байты картинки. Не участнику чата и несуществующей картинке
// отвечает одинаково, см. Repository.GetImage.
func (s *Server) GetImage(c *gin.Context) {

	imageID, err := strconv.Atoi(c.Param(imageIdKey))
	if err != nil {
		s.fail(c, errors1.ErrImageNotFound)
		return
	}

	image, err := s.service.GetImage(c.Request.Context(), imageID, c.GetInt(userIdKey))
	if err != nil {
		s.fail(c, err)
		return
	}

	c.Header("Cache-Control", imageCacheControl)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, image.ContentType, image.Data)
}

// readImageForm разбирает тело запроса, ограничив его размер. Тело больше
// лимита — ErrImageTooLarge, любой другой сбой разбора — некорректный запрос.
func readImageForm(c *gin.Context) (*multipart.Form, error) {

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)

	// Всё помещается в память: тело и так не больше maxRequestBytes.
	if err := c.Request.ParseMultipartForm(maxRequestBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, errors1.ErrImageTooLarge
		}
		return nil, errors1.ErrIncorrectData
	}

	return c.Request.MultipartForm, nil
}

// readImageFile читает файл из формы, не больше MaxImageSize+1 байт: лишний
// байт нужен, чтобы сервис заметил превышение.
func readImageFile(form *multipart.Form) ([]byte, error) {

	files := form.File[imageField]
	if len(files) == 0 {
		return nil, errors1.ErrNoImage
	}

	file, err := files[0].Open()
	if err != nil {
		return nil, errors1.ErrIncorrectData
	}
	defer func() {
		_ = file.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(file, service.MaxImageSize+1))
	if err != nil {
		return nil, errors1.ErrIncorrectData
	}

	return data, nil
}

// formValue читает текстовое поле формы. Запрос сюда не подмешивается: поля
// берутся только из тела.
func formValue(form *multipart.Form, name string) string {

	values := form.Value[name]
	if len(values) == 0 {
		return emptyField
	}

	return values[0]
}

// formID читает числовое поле формы. Нет поля или оно пустое — nullID, как
// отсутствующее поле в кадре send.
func formID(form *multipart.Form, name string) (int, error) {

	value := strings.TrimSpace(formValue(form, name))
	if value == emptyField {
		return nullID, nil
	}

	id, err := strconv.Atoi(value)
	if err != nil {
		return nullID, errors1.ErrBadChatID
	}

	return id, nil
}
