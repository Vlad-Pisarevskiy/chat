package service

const (
	minLoginLength = 3
	maxLoginLength = 32

	minPasswordLength = 8
	maxPasswordLength = 64
	maxPasswordBytes  = 72

	nullLength           = 0
	minNameLength        = 1
	maxNameLength        = 50
	maxDescriptionLength = 255
	maxBodyLength        = 4096

	// MaxImageSize — наибольший размер картинки: 5 МБ, то есть 5 МиБ.
	MaxImageSize = 5 << 20

	// Сколько байт хватает http.DetectContentType, чтобы узнать формат.
	sniffLength = 512

	contentTypeJPEG = "image/jpeg"
	contentTypePNG  = "image/png"

	emptyID   = 0
	emptyName = ""
	emptyHash
	emptyLogin
	emptyPassword
	emptyToken
	emptyBody
)
