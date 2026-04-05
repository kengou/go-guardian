package main

import (
	"fmt"
	"os"

	"golang.org/x/text/language"
)

func main() {
	// Use the vulnerable dependency so go mod tidy doesn't remove it.
	tag := language.English
	fmt.Println("Language:", tag)

	if len(os.Args) > 1 {
		result := HandleQuery(os.Args[1])
		fmt.Println(result)
	}
}
