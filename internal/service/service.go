package service

import (
	"chatflow/internal/model"
	"chatflow/internal/protocol"
	"chatflow/internal/repository/postgres"
	"context"
	rand "crypto/rand"
	"crypto/sha256"
	"slices"

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

	if err := s.validateRegister(ctx, input); err != nil {
		return err
	}

	hash, err := hashPassword(input.Password)
	if err != nil {
		return err
	}

	input.Password = hash

	user := model.User{
		Name:     input.Name,
		Login:    input.Login,
		Password: input.Password,
	}

	if err = s.db.RegisterUser(ctx, user); err != nil {
		return err
	}

	return nil
}

func (s *Service) LoginUser(ctx context.Context, login, password string) (*model.User, string, error) {

	if err := correctLogin(login); err != nil {
		return nil, emptyToken, err
	}

	user, err := s.db.FindUserByLogin(ctx, login)
	if err != nil {
		return nil, emptyToken, err
	}

	if err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, "", err
	}

	token := rand.Text()
	tokenHash := sha256.Sum256([]byte(token))

	if err = s.db.AddToken(ctx, user.ID, tokenHash[:]); err != nil {
		return nil, emptyToken, err
	}

	return user, token, err
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

func (s *Service) CreateGroup(ctx context.Context, group GroupCreate) (int, error) {

	group.Members = append(group.Members, group.OwnerID)
	slices.Sort(group.Members)
	group.Members = slices.Compact(group.Members)

	chatID, err := s.db.CreateGroup(ctx, group.Name, group.Members)
	if err != nil {
		return nullID, err
	}

	return chatID, err
}

func (s *Service) CreateChannel(ctx context.Context, ownerID int, channel ChannelCreates) (int, error) {

	channel.Name = strings.Trim(channel.Name, " ")
	if err := verifyChannelName(channel.Name); err != nil {
		return nullID, err
	}

	if channel.Handle != nil {
		if err := verifyChannelHandle(*channel.Handle); err != nil {
			return nullID, err
		}

	if len(channel.Name) > 100{
		return 0, //додделать
	}

	return s.db.CreateChannel(ctx, channel.OwnerID, channel.Name)
}

func (s *Service) GetGroups(ctx context.Context, userID int) ([]model.GroupFromDB, error) {

	return s.db.GetGroups(ctx, userID)
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

func (s *Service) GetOrCreateChat(ctx context.Context, peerID, sender int) (int, error) {

	chatID, ok, err := s.db.ChatExists(ctx, sender, peerID)
	if err != nil {
		return emptyID, err
	}

	if ok {
		return chatID, nil
	}

	chatID, err = s.db.StartChat(ctx, sender, peerID)
	if err != nil {
		return emptyID, err
	}

	return chatID, nil
}

func (s *Service) FindChat(ctx context.Context, userID, peerID int) (int, bool, error) {
	return s.db.ChatExists(ctx, userID, peerID)
}

func (s *Service) SendMessage(ctx context.Context, message protocol.Send, from int) (*protocol.Message, []int, error) {

	msg, userID, err := s.db.SendMessage(ctx, message, from)
	if err != nil {
		return nil, nil, err
	}

	return msg, userID, nil
}

func (s *Service) LoadMessages(ctx context.Context, chatID, from int) ([]protocol.Message, error) {

	return s.db.LoadMessages(ctx, chatID, from)
}
