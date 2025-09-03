package models

type User struct {
	ID   uint   `gorm:"primary_key" json:"id"`
	Name string `gorm:"name" json:"name"`
}
