package auth

type User struct {
	ID       int
	Name     string
	Login    string
	Password string
}

type RegisterInput struct {
	Name     string
	Login    string
	Password string
}

type UserFromDB struct {
	ID    int
	Name  string
	Login string
}
