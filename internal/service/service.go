package service

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/model"
	"chatflow/internal/protocol"
	"chatflow/internal/repository/postgres"
	"context"
	rand "crypto/rand"
	"crypto/sha256"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const nullID = 0

type Service struct {
	db *postgres.Repository
}

func New(repo *postgres.Repository) *Service {
	return &Service{db: repo}
}

func (s *Service) RegisterUser(ctx context.Context, input RegisterInput) error {

	input.Name = strings.TrimSpace(input.Name)

	if err := s.validateRegister(ctx, input); err != nil {
		return err
	}

	hash, err := hashPassword(input.Password)
	if err != nil {
		return err
	}

	user := model.User{
		Name:     input.Name,
		Login:    input.Login,
		Password: hash,
	}

	return s.db.RegisterUser(ctx, user)
}

func (s *Service) LoginUser(ctx context.Context, login, password string) (*model.User, string, error) {

	// Логин не по формату получает тот же ответ, что и неверный пароль: иначе по
	// тексту ошибки можно было бы отличить несуществующий логин от неверного пароля.
	if err := correctLogin(login); err != nil {
		return nil, emptyToken, errors1.ErrIncorrectLoginData
	}

	user, err := s.db.FindUserByLogin(ctx, login)
	if err != nil {
		return nil, emptyToken, err
	}

	if err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, emptyToken, errors1.ErrIncorrectLoginData
	}

	token := rand.Text()
	tokenHash := sha256.Sum256([]byte(token))

	if err = s.db.AddToken(ctx, user.ID, tokenHash[:]); err != nil {
		return nil, emptyToken, err
	}

	return user, token, nil
}

func (s *Service) CheckToken(ctx context.Context, token string) (int, error) {

	tokenHash := sha256.Sum256([]byte(token))
	id, err := s.db.CheckToken(ctx, tokenHash[:])
	if err != nil {
		return emptyID, err
	}

	return id, nil
}

func (s *Service) CleanupTokens(ctx context.Context) (int64, error) {

	return s.db.DeleteExpiredTokens(ctx)

}

func (s *Service) Logout(ctx context.Context, token string) error {

	tokenHash := sha256.Sum256([]byte(token))
	if err := s.db.RemoveToken(ctx, tokenHash[:]); err != nil {
		return err
	}

	return nil
}

func (s *Service) CreateGroup(ctx context.Context, group GroupCreate) (*CreatedGroup, error) {

	group.Name = strings.TrimSpace(group.Name)
	if err := verifyName(group.Name); err != nil {
		return nil, err
	}

	// Себя фронт в список не кладёт, но если id всё же пришёл, это не ошибка:
	// создатель добавляется ниже в любом случае. Дубли тоже не ошибка.
	members := slices.DeleteFunc(slices.Clone(group.Members), func(id int) bool {
		return id == group.OwnerID
	})
	slices.Sort(members)
	members = slices.Compact(members)

	if len(members) == nullLength {
		return nil, errors1.ErrNoMembers
	}

	members = append(members, group.OwnerID)

	chatID, err := s.db.CreateGroup(ctx, group.Name, members)
	if err != nil {
		return nil, err
	}

	return &CreatedGroup{ChatID: chatID, Name: group.Name, Members: members}, nil
}

// CreateChannel создаёт канал и возвращает его id и название после trim.
func (s *Service) CreateChannel(ctx context.Context, ownerID int, channel ChannelCreates) (int, string, error) {

	channel.Name = strings.TrimSpace(channel.Name)
	if err := verifyName(channel.Name); err != nil {
		return nullID, emptyName, err
	}

	if channel.Description != nil {
		description := strings.TrimSpace(*channel.Description)
		if err := verifyChannelDescription(description); err != nil {
			return nullID, emptyName, err
		}

		// Пустое описание — то же, что его отсутствие: у «нет описания» в базе
		// одно представление, NULL.
		channel.Description = &description
		if description == emptyName {
			channel.Description = nil
		}
	}

	if channel.Handle != nil {
		if err := verifyChannelHandle(*channel.Handle); err != nil {
			return nullID, emptyName, err
		}

		chatID, err := s.db.CreatePublicChannel(ctx, ownerID, channel.Name, channel.Description, *channel.Handle)
		return chatID, channel.Name, err
	}

	chatID, err := s.db.CreatePrivateChannel(ctx, ownerID, channel.Name, channel.Description)
	return chatID, channel.Name, err
}

func (s *Service) JoinChannel(ctx context.Context, join JoinChannel) (*model.ChannelFromDB, error) {

	return s.db.JoinChannel(ctx, join.UserID, join.ChannelID)
}

// DeleteChat удаляет личный чат у обоих и возвращает id его участников.
func (s *Service) DeleteChat(ctx context.Context, delete ChatDelete) ([]int, error) {

	return s.db.DeleteChat(ctx, delete.ChatID, delete.UserID)
}

func (s *Service) DeleteMessage(ctx context.Context, delete MessageDelete) error {

	return s.db.DeleteMessage(ctx, delete.UserID, delete.MessageID, delete.ChatID)
}

func (s *Service) GetGroups(ctx context.Context, userID int) ([]model.GroupFromDB, error) {

	return s.db.GetGroups(ctx, userID)
}

func (s *Service) GetChannels(ctx context.Context, userID int) ([]model.ChannelFromDB, error) {

	return s.db.GetChannels(ctx, userID)
}

func (s *Service) FindUserByID(ctx context.Context, id int) (*model.UserFromDB, error) {

	user, err := s.db.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) GetUsers(ctx context.Context) ([]*model.UserFromDB, error) {

	return s.db.GetUsers(ctx)

}

func (s *Service) GetUsersExcept(ctx context.Context, id int) ([]*model.UserFromDB, error) {

	return s.db.GetUsersExcept(ctx, id)
}

func (s *Service) ClearChat(ctx context.Context, chatID, userID int) (int, error) {

	return s.db.ClearChat(ctx, chatID, userID)
}

// GetOrCreateChat возвращает личный чат пары и сообщает, создан ли он сейчас.
func (s *Service) GetOrCreateChat(ctx context.Context, peerID, sender int) (int, bool, error) {

	return s.db.GetOrCreateDirect(ctx, sender, peerID)
}

func (s *Service) FindChat(ctx context.Context, userID, peerID int) (int, bool, error) {

	return s.db.FindDirect(ctx, userID, peerID)
}

// PrepareSend проверяет кадр send целиком до того, как что-то создано: личный
// чат по peer_id не должен появляться ради сообщения, которое не сохранится.
// Возвращает кадр с обрезанным body и client_msg_id в каноническом виде.
func (s *Service) PrepareSend(send protocol.Send, senderID int) (protocol.Send, error) {

	id, err := uuid.Parse(send.ClientMsgID)
	if err != nil {
		return send, errors1.ErrInvalidClientMsgID
	}
	send.ClientMsgID = id.String()

	if (send.ChatID == nullID) == (send.PeerID == nullID) {
		return send, errors1.ErrChatOrPeer
	}

	if send.PeerID == senderID {
		return send, errors1.ErrPeerIsSelf
	}

	send.Body = strings.TrimSpace(send.Body)
	if send.Body == emptyBody {
		return send, errors1.ErrEmptyBody
	}

	if utf8.RuneCountInString(send.Body) > maxBodyLength {
		return send, errors1.ErrLongBody
	}

	return send, nil
}

func (s *Service) SendMessage(ctx context.Context, message protocol.Send, from int) (*SentMessage, error) {

	msg, recipients, duplicate, err := s.db.SendMessage(ctx, message, from)
	if err != nil {
		return nil, err
	}

	return &SentMessage{Message: msg, Recipients: recipients, Duplicate: duplicate}, nil
}

func (s *Service) GetChannel(ctx context.Context, handle string) (*model.ChannelFromDB, error) {

	return s.db.GetChannel(ctx, handle)
}

func (s *Service) LoadMessages(ctx context.Context, chatID, from int) ([]protocol.Message, error) {

	return s.db.LoadMessages(ctx, chatID, from)
}
