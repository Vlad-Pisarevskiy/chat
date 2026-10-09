package app_errors

import "errors"

var ErrLoginNotFound = errors.New("login is not found in DB")

var ErrInvalidToken = errors.New("token does not exist")

var ErrEmptyLogin = errors.New("login is empty")

var ErrShortLogin = errors.New("login is too short")

var ErrLongLogin = errors.New("login is too long")

var ErrEmptyPassword = errors.New("password is empty")

var ErrShortPassword = errors.New("password is too short")

var ErrLongPassword = errors.New("password is too long")

var ErrEmptyName = errors.New("name is empty")

var ErrLongName = errors.New("name is too long")

var ErrExistsLogin = errors.New("login is already exists")

var ErrIncorrectLoginData = errors.New("login or password is incorrect")

var ErrEmptyDatabasePath = errors.New("empty database path")

var ErrIncorrectData = errors.New("incorrect data")

var ErrUserDoesntExist = errors.New("user doesnt exist")

var ErrNotChatMember = errors.New("user is not a chat member")

var ErrNameInUse = errors.New("channel name is already in use")

var ErrEmptyHandle = errors.New("handle is empty")

var ErrLongHandle = errors.New("handle is too long")

var ErrIncorrectNameLength = errors.New("name length is incorrect")

var ErrIncorrectMembersCount = errors.New("members can`t be less then 2")

var ErrTooLongDescription = errors.New("description is too long")

var ErrWrongChatID = errors.New("incorrect chat id")

var ErrIncorrectHandle = errors.New("handle does not exists")
