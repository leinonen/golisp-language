package main

import (
	"fmt"
)

func main() {
	fmt.Println(greet("World"))
	fmt.Println(greet("glisp"))
	nums := _glispMap(func(n any) any {
		return square(_glispToInt(n))
	}, _glispRange(1, 6))
	printResults("squares", nums)
}
