package model

type User struct {
	ID       int
	Name     string
	Login    string
	Password string
}

type UserFromDB struct {
	ID    int
	Name  string
	Login string
}

type UserFromAuth struct {
	ID    int
	Name  string
	Login string
}

type GroupFromDB struct {
	ID   int    `json:"group_id" db:"id"`
	Name string `json:"group_name" db:"name"`
}

type ChannelFromDB struct {
	ID          int    `json:"channel_id" db:"id"`
	Name        string `json:"name" db:"name"`
	Description string `json:"description,omitempty" db:"description"`
	Role        string `json:"role,omitempty" db:"role"`
}

// Image — картинка, которую отправитель приложил к сообщению. ContentType
// определён сервером по содержимому файла.
type Image struct {
	ContentType string
	Data        []byte
}
