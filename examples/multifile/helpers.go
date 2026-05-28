package main

import (
	"fmt"
)

func greet(name string) string {
	return (fmt.Sprintf("%v", "Hello, ") + fmt.Sprintf("%v", name) + fmt.Sprintf("%v", "!"))
}

func square(n int) int {
	return (n * n)
}

func printResults(label string, nums []any) {
	fmt.Printf("%s: %v\n", label, nums)
}
