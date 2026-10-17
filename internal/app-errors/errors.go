package app_errors

import "errors"

// Validation — ошибка во входных данных. Её текст написан для человека, и
// сервер отдаёт его как есть: HTTP-ручки с кодом 400, сокет — с bad_request.
type Validation struct {
	text string
}

func (e *Validation) Error() string {
	return e.text
}

func newValidation(text string) error {
	return &Validation{text: text}
}

// IsValidation сообщает, что ошибку вызвали входные данные, а не сбой сервера.
func IsValidation(err error) bool {
	var v *Validation
	return errors.As(err, &v)
}

// Входные данные.
var (
	ErrEmptyLogin          = newValidation("логин не указан")
	ErrShortLogin          = newValidation("логин короче 3 символов")
	ErrLongLogin           = newValidation("логин длиннее 32 символов")
	ErrLoginFormat         = newValidation("логин может состоять только из латинских букв, цифр и _")
	ErrEmptyPassword       = newValidation("пароль не указан")
	ErrShortPassword       = newValidation("пароль короче 8 символов")
	ErrLongPassword        = newValidation("пароль слишком длинный")
	ErrEmptyName           = newValidation("имя не указано")
	ErrLongName            = newValidation("имя длиннее 50 символов")
	ErrIncorrectData       = newValidation("некорректный запрос")
	ErrIncorrectNameLength = newValidation("название должно быть от 1 до 50 символов")
	ErrTooLongDescription  = newValidation("описание длиннее 255 символов")
	ErrInvalidHandle       = newValidation("имя канала: от 5 до 32 символов, латинские буквы, цифры и _, первая — буква")
	ErrNoMembers           = newValidation("выберите хотя бы одного участника")
	ErrUnknownMembers      = newValidation("среди участников есть несуществующий пользователь")
	ErrNotDirectChat       = newValidation("это работает только для личной переписки")
	ErrPeerIsSelf          = newValidation("нельзя написать самому себе")
	ErrBadFrame            = newValidation("кадр не разобран")
	ErrUnknownFrame        = newValidation("неизвестный тип кадра")
	ErrInvalidClientMsgID  = newValidation("client_msg_id должен быть UUID")
	ErrChatOrPeer          = newValidation("нужно ровно одно из полей chat_id и peer_id")
	ErrEmptyBody           = newValidation("пустое сообщение")
	ErrLongBody            = newValidation("сообщение длиннее 4096 символов")
	ErrBadChatID           = newValidation("chat_id и peer_id должны быть числами")
	ErrNoImage             = newValidation("картинка не приложена")
	ErrBadImageFormat      = newValidation("картинка должна быть в формате JPEG или PNG")
)

// Доступ и существование.
var (
	ErrIncorrectLoginData = errors.New("неверный логин или пароль")
	ErrInvalidToken       = errors.New("сессия не найдена или истекла")
	ErrExistsLogin        = errors.New("логин уже занят")
	ErrNameInUse          = errors.New("имя канала уже занято")
	ErrChatNotFound       = errors.New("чат не найден")
	ErrUserDoesntExist    = errors.New("пользователь не найден")
	ErrNotChatMember      = errors.New("вы не участник этого чата")
	ErrNoWriteAccess      = errors.New("в канал пишут только владелец и админы")
	ErrImageNotFound      = errors.New("картинка не найдена")
	ErrImageTooLarge      = errors.New("картинка больше 5 МБ")
)

var ErrEmptyDatabasePath = errors.New("не задан DATABASE_URL")
