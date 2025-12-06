package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println("Usage: greeter [name]")
		return
	}
	fmt.Println("Hello, World")
}
