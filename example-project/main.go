package main

import "fmt"

// Message returns the greeting used by the example CLI.
func Message() string {
	return "example-project: hello from pt demo"
}

func main() {
	fmt.Println(Message())
}
