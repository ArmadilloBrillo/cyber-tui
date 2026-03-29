package model

import "time"

type User struct {
	ID       string
	Username string
	Bio      string
}

type Post struct {
	ID        string
	Author    User
	Body      string
	CreatedAt time.Time
	Topics    []string
}

type Message struct {
	ID        string
	From      User
	Body      string
	CreatedAt time.Time
}

type Conversation struct {
	ID           string
	Participants []User
	Messages     []Message
}

type Room struct {
	ID          string
	Name        string
	Description string
	Members     int
}
