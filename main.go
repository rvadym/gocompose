package main

import (
	"log"

	"github.com/rvadym/gocompose/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}
