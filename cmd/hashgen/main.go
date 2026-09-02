package main

import (
	"fmt"
	"log"
	"sigardapanel/internal/auth"
)

func main() {
	hash, err := auth.HashPassword("test123")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(hash)
}
