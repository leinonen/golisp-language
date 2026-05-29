package main

import (
	"fmt"
	"strings"
)

func grade(score int) string {
	if score >= 90 {
		return "A"
	} else if score >= 80 {
		return "B"
	} else if score >= 70 {
		return "C"
	} else if score >= 60 {
		return "D"
	} else {
		return "F"
	}
}

func fib(n int) int {
	i := n
	a := 0
	b := 1
	for {
		if i <= 0 {
			return a
		} else {
			_r0 := (i - 1)
			_r1 := b
			_r2 := (a + b)
			i = _r0
			a = _r1
			b = _r2
			continue
		}
	}
}

func longWords(text string) []any {
	ws := _glispSplit(strings.ToLower(_glispToString(strings.TrimSpace(_glispToString(text)))), " ")
	return _glispSortBy(func(w any) any {
		return w
	}, _glispFilter(func(w any) any {
		return (len((fmt.Sprintf("%v", w))) > 3)
	}, ws))
}

func main() {
	fibs := _glispMap(func(n any) any {
		return fib(_glispToInt(n))
	}, _glispRange(10))
	fmt.Println("fib(0..9):", _glispJoin(_glispMap(func(n any) any {
		return (fmt.Sprintf("%v", n))
	}, fibs), " "))
	words := longWords("the quick brown fox jumps over the lazy dog")
	fmt.Println("long words:", _glispJoin(words, ", "))
	classes := []any{map[string]any{"name": "Math", "scores": []any{85, 92, 78, 61, 79, 88, 95}}, map[string]any{"name": "English", "scores": []any{76, 85, 90, 55, 68, 73, 82}}, map[string]any{"name": "Science", "scores": []any{91, 94, 87, 98, 79, 85, 92}}}
	ch := make(chan map[string]any, len(classes))
	_glispMap(func(_p1 any) any {
		name := _glispGet(_p1, "name")
		scores := _glispGet(_p1, "scores")
		total := _glispReduce(func(acc any, s any) any {
			return (_glispToInt(acc) + _glispToInt(s))
		}, 0, scores)
		n := _glispToInt(_glispReduce(func(k any, _ any) any {
			return (_glispToInt(k) + 1)
		}, 0, scores))
		avg := (_glispToInt(total) / n)
		pass := _glispFilter(func(s any) any {
			return (_glispToInt(s) >= 60)
		}, scores)
		passN := _glispToInt(_glispReduce(func(k any, _ any) any {
			return (_glispToInt(k) + 1)
		}, 0, pass))
		go func() {
			ch <- map[string]any{"name": (fmt.Sprintf("%v", name)), "avg": avg, "grade": grade(avg), "passing": (fmt.Sprintf("%v", passN) + fmt.Sprintf("%v", "/") + fmt.Sprintf("%v", n))}
		}()
		return nil
	}, classes)
	results := _glispMap(func(_ any) any {
		return <-ch
	}, classes)
	ranked := _glispSortBy(func(_p2 any) any {
		avg := _glispGet(_p2, "avg")
		return avg
	}, results)
	_v3 := ranked
	top := _glispGet(_v3, int64(0))
	second := _glispGet(_v3, int64(1))
	third := _glispGet(_v3, int64(2))
	fmt.Println("\n=== Class Report ===")
	_glispMap(func(_p4 any) any {
		name := _glispGet(_p4, "name")
		avg := _glispGet(_p4, "avg")
		grade := _glispGet(_p4, "grade")
		passing := _glispGet(_p4, "passing")
		fmt.Println(" ", name, "| avg:", avg, "| grade:", grade, "| passing:", passing)
		return nil
	}, ranked)
	fmt.Println("\nbottom:", _glispGet(top, "name"), " mid:", _glispGet(second, "name"), " top:", _glispGet(third, "name"))
	func() any {
		out, err := _glispJsonEncode(map[string]any{"report": ranked})
		if err != nil {
			fmt.Println("encode error:", err)
			return nil
		}
		fmt.Println("\nJSON:", out)
		return nil
	}()
}
