package main

import (
	"fmt"
	"math"
)

type Shape interface {
	Area() float64
	Describe() string
}

type Circle struct {
	Radius float64
}

func (c Circle) Area() float64 {
	return (math.Pi * c.Radius * c.Radius)
}

func (c Circle) Describe() string {
	return fmt.Sprintf("circle(r=%.2f)", c.Radius)
}

type Rect struct {
	Width  float64
	Height float64
}

func (r Rect) Area() float64 {
	return (r.Width * r.Height)
}

func (r Rect) Describe() string {
	return fmt.Sprintf("rect(%.2f x %.2f)", r.Width, r.Height)
}

type Triangle struct {
	A float64
	B float64
	C float64
}

func (t Triangle) Area() float64 {
	s := (0.5 * (t.A + t.B + t.C))
	return math.Sqrt((s * (s - t.A) * (s - t.B) * (s - t.C)))
}

func (t Triangle) Describe() string {
	return fmt.Sprintf("triangle(%.2f, %.2f, %.2f)", t.A, t.B, t.C)
}

func printShape(s Shape) {
	fmt.Printf("  %-28s  area = %.4f\n", s.Describe(), s.Area())
}

func main() {
	fmt.Println("Shapes:")
	printShape(Circle{Radius: 5})
	printShape(Rect{Width: 4, Height: 6})
	printShape(Triangle{A: 3, B: 4, C: 5})
}
