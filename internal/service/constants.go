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

	emptyID   = 0
	emptyName = ""
	emptyHash
	emptyLogin
	emptyPassword
	emptyToken
	emptyBody
)
