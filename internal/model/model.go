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

type GroupFromDB struct {
	ID   int    `json:"group_id" db:"id"`
	Name string `json:"group_name" db:"label"`
}

type ChannelFromDB struct {
	ID   int    `json:"channel_id" db:"id"`
	Name string `json:"name" db:"name"`
}
